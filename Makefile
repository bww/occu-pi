
GOPATH  := $(PWD)
PRODUCT := $(PWD)/bin/occupi
SRC     := $(shell find src -name \*.go -print)

all: build

build: $(PRODUCT)

$(PRODUCT): $(SRC)
	mkdir -p $(PWD)/bin && go build -o $(PRODUCT) occupi/main

