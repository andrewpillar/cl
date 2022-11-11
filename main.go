package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type host struct {
	user     string
	addr     string
	identity string
}

func parseClFile(r io.Reader) map[string][]host {
	s := bufio.NewScanner(r)
	m := make(map[string][]host)

	curr := ""

	for s.Scan() {
		line := s.Text()

		if line == "" || line[0] == '#' {
			continue
		}

		end := len(line)-1

		if line[end] == ':' {
			curr = line[:end]
			continue
		}

		pos := 0

		for i, r := range line {
			if r != ' ' && r != '\t' {
				pos = i
				break
			}
		}

		line = line[pos:]

		h := host{
			user:     os.Getenv("USER"),
			identity: filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa"),
		}

		if _, ok := m[curr]; !ok {
			m[curr] = make([]host, 0)
		}

		if strings.Contains(line, " ") {
			parts := strings.Fields(line)

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
	key, err := os.ReadFile(h.identity)

	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)

	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User: h.user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(time.Second * 60),
	}

	conn, err := ssh.Dial("tcp", h.addr, cfg)

	if err != nil {
		return nil, err
	}

	defer conn.Close()

	sess, err := conn.NewSession()

	if err != nil {
		return nil, err
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
	argv0 := os.Args[0]

	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <cluster> <commands...>\n", argv0)
		os.Exit(1)
	}

	f, err := os.Open("ClFile")

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", argv0, err)
		os.Exit(1)
	}

	defer f.Close()

	cluster := parseClFile(f)

	hosts, ok := cluster[os.Args[1]]

	if !ok {
		fmt.Fprintf(os.Stderr, "%s: unknown cluster\n", argv0)
		os.Exit(1)
	}

	var wg sync.WaitGroup

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
				return
			}

			out <- append([]byte("Host: " + h.addr + "\n"), b...)
		}(h, cmd)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	go func() {
		<-ch
		cancel()
	}()

	go func() {
		wg.Wait()

		close(errs)
		close(out)
	}()

	line := make([]byte, 0)
	code := 0

	for errs != nil && out != nil {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "%s: %s\n", argv0, ctx.Err())
			err = nil
			out = nil
			break
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

			for _, b := range p[i:] {
				line = append(line, b)

				if b == '\n' {
					os.Stdout.Write(append([]byte("  "), line...))
					line = line[0:0]
				}
			}
		}
	}

	if code != 0 {
		os.Exit(code)
	}
}
