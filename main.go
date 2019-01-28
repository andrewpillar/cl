package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/andrewpillar/cli"

	"github.com/kevinburke/ssh_config"

	"golang.org/x/crypto/ssh"

	"gopkg.in/yaml.v2"
)

var (
	sshConfig     = filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	sshKnownHosts = filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
)

type Cl map[string][]string

func exitError(err error) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err)
	os.Exit(1)
}

func getHostKey(host string) (ssh.PublicKey, error) {
	f, err := os.Open(sshKnownHosts)

	if err != nil {
		return nil, err
	}

	defer f.Close()

	var hostKey ssh.PublicKey

	s := bufio.NewScanner(f)

	for s.Scan() {
		fields := strings.Split(s.Text(), " ")

		if len(fields) != 3 {
			continue
		}

		if strings.Contains(fields[0], host) {
			var err error

			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(s.Bytes())

			if err != nil {
				return nil, err
			}

			break
		}
	}

	if hostKey == nil {
		return nil, errors.New("host " + host + " not in " + sshKnownHosts)
	}

	return hostKey, nil
}

func mainCommand(c cli.Command) {
	if len(c.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: cl [cluster] [command...] [-c file]\n")
		os.Exit(1)
	}

	fcl, err := os.Open(c.Flags.GetString("config"))

	if err != nil {
		exitError(err)
	}

	defer fcl.Close()

	cl := Cl(make(map[string][]string))

	dec := yaml.NewDecoder(fcl)

	if err := dec.Decode(&cl); err != nil {
		exitError(err)
	}

	name := c.Args.Get(0)

	hosts, ok := cl[name]

	if !ok {
		exitError(errors.New("unknown cluster " + name))
	}

	var cfg *ssh_config.Config

	fcfg, err := os.Open(sshConfig)

	if err != nil && !os.IsNotExist(err) {
		exitError(err)
	}

	if err == nil {
		defer fcfg.Close()

		cfg, err = ssh_config.Decode(fcfg)

		if err != nil {
			exitError(err)
		}
	}

	wg := &sync.WaitGroup{}
	mut := &sync.Mutex{}

	errs := make(chan error)
	stdout := make(chan string)
	stderr := make(chan string)

	cmd := strings.Join(c.Args[1:], " ")

	user := os.Getenv("USER")
	identity := filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")

	for _, addr := range hosts {
		wg.Add(1)

		go func(addr string) {
			defer wg.Done()

			if strings.Contains(addr, "@") {
				parts := strings.Split(addr, "@")

				user = parts[0]
				addr = parts[1]
			}

			host, _, err := net.SplitHostPort(addr)

			if err != nil {
				errs <- err
				return
			}

			if cfg != nil {
				mut.Lock()
				identity, err = cfg.Get(host, "IdentityFile")
				mut.Unlock()

				if err != nil {
					errs <- err
					return
				}
			}

			key, err := ioutil.ReadFile(identity)

			if err != nil {
				errs <- err
				return
			}

			signer, err := ssh.ParsePrivateKey(key)

			if err != nil {
				errs <- err
				return
			}

			mut.Lock()
			hostKey, err := getHostKey(host)
			mut.Unlock()

			if err != nil {
				errs <- err
				return
			}

			clientCfg := &ssh.ClientConfig{
				User: user,
				Auth: []ssh.AuthMethod{
					ssh.PublicKeys(signer),
				},
				HostKeyCallback: ssh.FixedHostKey(hostKey),
			}

			conn, err := ssh.Dial("tcp", addr, clientCfg)

			if err != nil {
				errs <- err
				return
			}

			defer conn.Close()

			sess, err := conn.NewSession()

			if err != nil {
				errs <- err
				return
			}

			defer sess.Close()

			stdoutBuf := &bytes.Buffer{}
			stderrBuf := &bytes.Buffer{}

			sess.Stdout = stdoutBuf
			sess.Stderr = stderrBuf

			if err := sess.Run(cmd); err != nil {
				if _, ok := err.(*ssh.ExitError); !ok {
					errs <- err
					return
				}
			}

			stdout <- user + "@" + addr + ": " + stdoutBuf.String()
			stderr <- user + "@" + addr + ": " + stderrBuf.String()
		}(addr)
	}

	go func() {
		wg.Wait()

		close(errs)
		close(stdout)
		close(stderr)
	}()

	for errs != nil && stdout != nil && stderr != nil {
		select {
			case err, ok := <-errs:
				if !ok {
					errs = nil
				} else {
					fmt.Fprintf(os.Stderr, "%s: %s\n", os.Args[0], err)
				}
			case out, ok := <-stdout:
				if !ok {
					stdout = nil
				} else {
					fmt.Fprintf(os.Stdout, "%s\n", out)
				}
			case err, ok := <-stderr:
				if !ok {
					stderr = nil
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", err)
				}
		}
	}
}

func main() {
	c := cli.New()

	c.Main(mainCommand).AddFlag(&cli.Flag{
		Name:     "config",
		Short:    "-c",
		Long:     "--config",
		Argument: true,
		Default:  "cl.yml",
	})

	if err := c.Run(os.Args[1:]); err != nil {
		exitError(err)
	}
}
