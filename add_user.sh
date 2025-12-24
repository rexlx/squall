#!/bin/bash

go run ./cmd/admin-cli/main.go \
  -admin admin@example.com \
  -pass secret \
  -new-email user2@example.com \
  -new-pass user2pass \
  -new-name "Second User" \
  -new-role user
