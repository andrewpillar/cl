package main

import (
	"strings"
	"testing"
)

func Test_parseClFile(t *testing.T) {
	clfile := `
prod:
	prod0.example.com ~/.ssh/id_ed25519
	prod1.example.com ~/.ssh/id_ed25519
	prod2.example.com ~/.ssh/id_ed25519
uat:
    uat0.example.com 	~/.ssh/id_rsa
    uat1.example.com 	  	~/.ssh/id_rsa
    uat2.example.com   	~/.ssh/id_rsa

# Comment line, this is ignored.
mixed:
	mixed0.example.com
	    mixed1.example.com:444
  mixed2.example.com
                  		 	 	    mixed3.example.com 		 	 	   ~/.ssh/id_ed25519
`

	envs := parseClFile(strings.NewReader(clfile))

	tests := []struct{
		key   string
		count int
	}{
		{"prod", 3},
		{"uat", 3},
		{"mixed", 4},
	}

	for i, test := range tests {
		hosts, ok := envs[test.key]

		if !ok {
			t.Fatalf("tests[%d] - expected key %q in map, it was not\n", i, test.key)
		}

		if len(hosts) != test.count {
			t.Fatalf("tests[%d] - unexpected host count, expected=%d, got=%d\n", i, test.count, len(hosts))
		}
	}
}
