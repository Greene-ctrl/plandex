FROM golang:1.23.3

# Update and install necessary packages including build tools for Tree-sitter and Postgres
RUN apt-get update && \
  apt-get install -y git gcc g++ make python3 python3-venv postgresql postgresql-contrib

# Install Python and create a virtual environment for litellm passthrough
RUN python3 -m venv /opt/venv

# Activate the virtual environment for all following RUN commands
ENV PATH="/opt/venv/bin:$PATH"

# Now install litellm passthrough dependencies in the virtual environment
RUN pip install --no-cache-dir "litellm==1.72.6" "fastapi==0.115.12" "uvicorn==0.34.1" "google-cloud-aiplatform==1.96.0" "boto3==1.38.40" "botocore==1.38.40"

WORKDIR /app

# Copy go.mod and go.sum for shared and server, and install dependencies
COPY ./app/shared/go.mod ./app/shared/go.sum ./app/shared/
RUN cd app/shared && go mod download

COPY ./app/server/go.mod ./app/server/go.sum ./app/server/
RUN cd app/server && go mod download

# Copy the actual source code
COPY ./app/server ./app/server
COPY ./app/shared ./app/shared
COPY ./app/scripts /scripts

# Set working directory to server
WORKDIR /app/app/server

# Build the application
RUN rm -f plandex-server && go build -o plandex-server .

# Setup entrypoint script to start postgres and the server
RUN echo '#!/bin/bash\n\
service postgresql start\n\
su - postgres -c "psql -c \"CREATE USER plandex WITH PASSWORD '\''plandex'\'';\""\n\
su - postgres -c "psql -c \"CREATE DATABASE plandex OWNER plandex;\""\n\
export DATABASE_URL="postgres://plandex:plandex@localhost:5432/plandex?sslmode=disable"\n\
export GOENV=development\n\
export LOCAL_MODE=1\n\
export PLANDEX_BASE_DIR=/plandex-server\n\
mkdir -p /plandex-server\n\
./plandex-server' > /app/entrypoint.sh && chmod +x /app/entrypoint.sh

# Set the port and expose it
ENV PORT=7860
EXPOSE 7860

# Command to run the entrypoint script
CMD ["/app/entrypoint.sh"]
