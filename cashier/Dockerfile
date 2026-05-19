FROM ubuntu:24.04 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    ninja-build \
    git \
    curl \
    zip \
    unzip \
    pkg-config \
    ca-certificates \
    libssl-dev \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

ENV VCPKG_ROOT=/opt/vcpkg
RUN git clone https://github.com/microsoft/vcpkg.git $VCPKG_ROOT && \
    $VCPKG_ROOT/bootstrap-vcpkg.sh -disableMetrics

ARG JOBS=6

COPY . /src
WORKDIR /src

RUN cmake --preset release -B build && \
    cmake --build build --parallel ${JOBS}

# ── Unit test stage ─────────────────────────────────────────────────────────
FROM builder AS test
RUN ctest --test-dir /src/build --output-on-failure -R cashier_test

# ── Integration test stage ──────────────────────────────────────────────────
FROM ubuntu:24.04 AS integration-test

RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/build/cashier_integration_test /usr/local/bin/cashier_integration_test

ENTRYPOINT ["/usr/local/bin/cashier_integration_test"]

# ── Runtime stage ───────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/build/cashier_server /usr/local/bin/cashier_server

ENV CASHIER_LISTEN_ADDR=0.0.0.0:50051
EXPOSE 50051

ENTRYPOINT ["/usr/local/bin/cashier_server"]

# ── Migrate stage ───────────────────────────────────────────────────────────
FROM liquibase/liquibase:4.27 AS migrate

COPY db/ /liquibase/changelog/
