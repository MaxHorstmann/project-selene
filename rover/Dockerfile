FROM golang:1.22-bookworm AS go-builder
WORKDIR /build
COPY src/mapper/ .
RUN go mod download
RUN go build -o mapper .

FROM python:3.12-slim AS base
RUN pip install --no-cache-dir fastapi uvicorn
RUN mkdir -p /rover/output
COPY base/server.py /rover/server.py
WORKDIR /rover
EXPOSE 8080

FROM base

# --- Install your dependencies below ---
# install some tools for debugging
RUN apt-get update && apt-get install -y curl jq iputils-ping iproute2 nmap
# Example: RUN pip install anthropic httpx networkx
# Example: RUN npm install -g tsx && npm install openai

# --- Copy your code ---
COPY . /rover/
COPY --from=go-builder /build/mapper /rover/mapper

# Make scripts executable
RUN chmod +x /rover/run_mapping.sh /rover/run_reporting.sh

CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8080"]
