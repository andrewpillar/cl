TAGS := "netgo"

all:
	go build -tags $(TAGS) -o cl.out
