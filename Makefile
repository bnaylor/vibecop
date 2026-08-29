BINARY := vibecop
CMD    := ./cmd/vibecop

.PHONY: build clean

build:
	go build -o $(BINARY) $(CMD)
	@if [ "$$(uname -s)" = "Darwin" ]; then codesign --sign - $(BINARY); fi

clean:
	rm -f $(BINARY)
