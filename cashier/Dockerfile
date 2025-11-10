# Multi-stage Docker build for SecWager Cashier Service

# Build stage - Using Alpine for smaller image
FROM alpine:3.19 AS builder

# Install system dependencies for building
RUN apk add --no-cache \
    build-base \
    cmake \
    ninja \
    git \
    curl \
    zip \
    unzip \
    tar \
    pkgconfig \
    perl \
    linux-headers \
    gcompat \
    postgresql-dev \
    openssl-dev \
    zlib-dev \
    readline-dev \
    bash \
    bison \
    flex \
    autoconf \
    automake \
    libtool \
    m4 \
    make \
    gettext-dev \
    krb5-dev \
    util-linux-dev \
    coreutils

# Install vcpkg
RUN git clone https://github.com/Microsoft/vcpkg.git /opt/vcpkg
RUN /opt/vcpkg/bootstrap-vcpkg.sh

# Set vcpkg environment
ENV VCPKG_ROOT=/opt/vcpkg
ENV PATH="/opt/vcpkg:${PATH}"

# Copy the entire project context
WORKDIR /app
COPY . .

# Work from cashier directory which has its own vcpkg.json
WORKDIR /app/cashier

# Configure build environment variables
ENV CC=gcc
ENV CXX=g++

# Build directly from cashier directory (has its own vcpkg.json)
RUN rm -rf build


# Configure and build from cashier directory
RUN cmake -S . -B build -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE=/opt/vcpkg/scripts/buildsystems/vcpkg.cmake \
    -DCMAKE_BUILD_TYPE=Release
RUN cmake --build build --target cashier -j$(nproc)

# Runtime stage - Ultra-lightweight Alpine
FROM alpine:3.19 AS runtime

# Install minimal runtime dependencies
RUN apk add --no-cache \
    libstdc++ \
    libgcc \
    curl \
    gcompat

# Create non-root user (Alpine Linux uses adduser)
RUN adduser -D -s /bin/false -h /opt/cashier cashier

# Create directories
RUN mkdir -p /opt/cashier/database
RUN mkdir -p /opt/cashier/logs

# Copy built executable
COPY --from=builder /app/build/cashier /usr/local/bin/cashier

# Copy database migrations
COPY --from=builder /app/cashier/database /opt/cashier/database

# Set permissions
RUN chown -R cashier:cashier /opt/cashier
RUN chmod +x /usr/local/bin/cashier

# Switch to non-root user
USER cashier

# Set working directory
WORKDIR /opt/cashier

# Expose port 
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

# Environment variables with defaults

ENV SERVER_PORT=8080

# Run the service
CMD ["/usr/local/bin/cashier"]