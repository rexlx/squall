#!/bin/bash

go run ./cmd/admin-cli/main.go \
  -admin rex@aol.com \
  -pass admin \
  -new-email user@aol.com \
  -new-pass admin \
  -new-name "Second User" \
  -new-role user
