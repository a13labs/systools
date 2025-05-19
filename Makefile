# Makefile for Go project

# Variables
BUILD_DIR = build
DIST_DIR = dist
PROJECTS = k8s_encryption_provider \
		   read_system_id \
		   open_volume \
		   k8s_gitea_auth \
		   k8s_gitea_shell \
		   ssh_locker \
		   ssh_locker_cli \
		   ssh_locker_web 

.PHONY: all build

all: clean dist

build:
	@echo "Building projects..."
	@echo "Building for Linux AMD64..."
	@mkdir -p $(BUILD_DIR) 
	@for project in $(PROJECTS); do \
		cd $$project; \
		GOOS=linux GOARCH=amd64 go build -o ../$(BUILD_DIR)/$$project .; \
		cd ..; \
	done

dist: clean build
	@echo "Creating distribution package..."
	@mkdir -p $(DIST_DIR)
	@cd $(BUILD_DIR); \
	 for project in $(PROJECTS); do \
		zip -r ../$(DIST_DIR)/systools-linux-amd64.zip $$project; \
	 done

clean:
	rm -rf $(BUILD_DIR)