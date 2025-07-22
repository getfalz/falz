BIN_DIR := bin

all: craeft gendoc

craeft:
	go build -o $(BIN_DIR)/craeft ./cmd/craeft

gendoc:
	go build -o $(BIN_DIR)/gendoc ./cmd/gendoc

prod:
	go build -ldflags="-s -w" -trimpath -o $(BIN_DIR)/craeft ./cmd/craeft

clean:
	rm -rf $(BIN_DIR)
