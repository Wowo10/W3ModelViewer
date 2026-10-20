.PHONY: build run

build:
	go build -o modelViewer .

run: build
	./modelViewer $(ARGS)
