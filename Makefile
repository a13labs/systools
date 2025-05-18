# Makefile for Go project

# Variables
BUILD_DIR = build
DIST_DIR = dist
PROJECTS = k8s_encryption_provider read_system_id open_volume

.PHONY: all build

all: clean dist

build: 
	for project in $(PROJECTS); do \
		cd $$project; \
		GOOS=linux GOARCH=amd64 go build -o ../$(BUILD_DIR)/$$project .; \
		cd ..; \
	done

dist: clean build
	mkdir -p $(DIST_DIR)
	for project in $(PROJECTS); do \
		zip -r $(DIST_DIR)/systools-linux-amd64.zip $(BUILD_DIR)/$$project; \
	done

clean:
	rm -rf $(BUILD_DIR)