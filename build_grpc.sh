#!/bin/bash

PACKAGE=$(head -1 go.mod | awk '{print $2}')

protoc -Iproto/ --go_opt=module=${PACKAGE} --go_out=. --go-grpc_opt=module=${PACKAGE} --go-grpc_out=. proto/*.proto
