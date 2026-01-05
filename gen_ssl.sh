#!/bin/bash

# --- 1. Configuration Variables ---
COUNTRY="US"
STATE="Texas"
CITY="Tomball"
ORG="Squall Dev"
UNIT="Engineering"
EMAIL="admin@squall.local"

# REPLACE THIS with your actual Server IP Address
SERVER_IP="192.168.1.50" 

# Directory to store the certs
OUT_DIR="data"

# -----------------------------------------------

mkdir -p $OUT_DIR
cd $OUT_DIR

echo "--- Generating Certificate Authority (CA) ---"
openssl req -x509 -new -nodes -days 3650 \
  -keyout ca-key.pem \
  -out ca-cert.pem \
  -subj "/C=$COUNTRY/ST=$STATE/L=$CITY/O=$ORG/OU=$UNIT/CN=Squall-Root-CA/emailAddress=$EMAIL"

echo "--- Generating Server Key and CSR ---"
# We use the IP as the Common Name (CN) here, though modern clients look at SANs.
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server-req.pem \
  -subj "/C=$COUNTRY/ST=$STATE/L=$CITY/O=$ORG/OU=$UNIT/CN=$SERVER_IP/emailAddress=$EMAIL"

echo "--- Signing Server Certificate (with IP SANs) ---"
# CRITICAL CHANGE: Note the "IP:$SERVER_IP" in the subjectAltName below.
# This tells TLS clients that this certificate is valid for that specific IP address.
openssl x509 -req -in server-req.pem -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out server-cert.pem -days 365 -sha256 \
  -extfile <(printf "subjectAltName=IP:$SERVER_IP,DNS:localhost,IP:127.0.0.1")

echo "--- Generating Client Key and CSR ---"
# Clients don't strictly need IP SANs, but we generate a valid client cert anyway.
openssl genrsa -out client-key.pem 4096
openssl req -new -key client-key.pem -out client-req.pem \
  -subj "/C=$COUNTRY/ST=$STATE/L=$CITY/O=$ORG/OU=$UNIT/CN=Squall-Client-User/emailAddress=$EMAIL"

echo "--- Signing Client Certificate ---"
openssl x509 -req -in client-req.pem -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out client-cert.pem -days 365 -sha256

echo "--- Cleanup ---"
rm server-req.pem client-req.pem

echo "Done! Certificates generated in $OUT_DIR/ for IP: $SERVER_IP"
