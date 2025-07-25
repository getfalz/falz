BIN_DIR := bin

.PHONY: craeft gendoc generate all prod clean

all: craeft gendoc

craeft: generate gendoc
	go build -o $(BIN_DIR)/craeft ./cmd/craeft

generate:
	go generate craeft/cmd/craeft

gendoc:
	go build -o $(BIN_DIR)/gendoc ./cmd/gendoc

prod:
	go build -ldflags="-s -w" -trimpath -o $(BIN_DIR)/craeft ./cmd/craeft

clean:
	rm -rf $(BIN_DIR)
