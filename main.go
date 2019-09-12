package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

var argv0 string

type host struct {
	user     string
	addr     string
	identity string
}

func unmarshal(r io.Reader) map[string][]host {
	s := bufio.NewScanner(r)
	m := make(map[string][]host)

	curr := ""

	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		if line == "" {
			continue
		}

		end := len(line) - 1

		if line[end] == ':' {
			curr = line[:end]
			continue
		}

		h := host{
			user:     os.Getenv("USER"),
			identity: filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa"),
		}

		if _, ok := m[curr]; !ok {
			m[curr] = make([]host, 0)
		}

		if strings.Contains(line, " ") {
			parts := strings.Split(line, " ")

			h.identity = parts[1]

			if h.identity[0] == '~' {
				h.identity = strings.Replace(h.identity, "~", os.Getenv("HOME"), 1)
			}

			line = parts[0]
		}

		if strings.Contains(line, "@") {
			parts := strings.Split(line, "@")

			h.user = parts[0]
			line = parts[1]
		}

		host, port, _ := net.SplitHostPort(line)

		if host == "" {
			host = line
		}

		if port == "" {
			port = "22"
		}

		h.addr = net.JoinHostPort(host, port)

		m[curr] = append(m[curr], h)
	}

	return m
}

func run(h host, cmd string) ([]byte, error) {
	key, err := ioutil.ReadFile(h.identity)

	if err != nil {
		return []byte{}, err
	}

	signer, err := ssh.ParsePrivateKey(key)

	if err != nil {
		return []byte{}, err
	}

	cfg := &ssh.ClientConfig{
		User: h.user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", h.addr, cfg)

	if err != nil {
		return []byte{}, err
	}

	defer conn.Close()

	sess, err := conn.NewSession()

	if err != nil {
		return []byte{}, err
	}

	defer sess.Close()

	b, err := sess.CombinedOutput(cmd)

	if err != nil {
		if _, ok := err.(*ssh.ExitError); !ok {
			return b, err
		}
	}

	return b, nil
}

func main() {
	argv0 = os.Args[0]

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: cl [cluster] [commands...]\n")
		os.Exit(1)
	}

	f, err := os.Open("ClFile")

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", argv0, err)
		os.Exit(1)
	}

	defer f.Close()

	cluster := unmarshal(f)

	hosts, ok := cluster[os.Args[1]]

	if !ok {
		fmt.Fprintf(os.Stderr, "%s: unknown cluster\n", argv0)
		os.Exit(1)
	}

	wg := &sync.WaitGroup{}
	cmd := strings.Join(os.Args[2:], " ")

	errs := make(chan error)
	out := make(chan []byte)

	for _, h := range hosts {
		wg.Add(1)

		go func(h host, cmd string) {
			defer wg.Done()

			b, err := run(h, cmd)

			if err != nil {
				errs <- err
			}

			hname, port, _ := net.SplitHostPort(h.addr)

			s := fmt.Sprintf(
				"ssh -i %s -p %s -l %s %s %s\n",
				h.identity,
				port,
				h.user,
				hname,
				cmd,
			)

			out <- append([]byte(s), b...)
		}(h, cmd)
	}

	go func() {
		wg.Wait()

		close(errs)
		close(out)
	}()

	code := 0

	for errs != nil && out != nil {
		select {
		case err, ok := <-errs:
			if !ok {
				errs = nil
				break
			}

			code = 1

			fmt.Fprintf(os.Stderr, "%s: %s\n", argv0, err)
			break
		case p, ok := <-out:
			if !ok {
				out = nil
				break
			}

			i := bytes.Index(p, []byte("\n"))

			os.Stderr.Write(p[:i])

			line := make([]byte, 0)

			for _, b := range p[i:] {
				line = append(line, b)

				if b == '\n' {
					os.Stdout.Write(append([]byte("  "), line...))
					line = make([]byte, 0)
				}
			}
		}
	}

	os.Exit(code)
}
