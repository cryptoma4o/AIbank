.PHONY: generate lint breaking clean

generate:
	buf generate

lint:
	buf lint

breaking:
	buf breaking --against '.git#branch=main'

clean:
	find . -name '*.pb.go' -delete
	find . -name '*.pb.ts' -delete
