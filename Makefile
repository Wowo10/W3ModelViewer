.PHONY: build run web

build:
	go build -o modelViewer .

run: build
	./modelViewer $(ARGS)

web: build
	./modelViewer -web $(ARGS)
