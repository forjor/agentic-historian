BINARY := historian-shell
GO     := go

.PHONY: build run clean

build:
	$(GO) build -o $(BINARY) .

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
