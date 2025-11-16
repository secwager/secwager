FROM alpine:3.19 AS builder



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
    linux-headers \
    bash \
    perl \
    gcompat \
    libc6-compat

RUN git clone https://github.com/Microsoft/vcpkg.git /opt/vcpkg
RUN /opt/vcpkg/bootstrap-vcpkg.sh

ENV VCPKG_ROOT=/opt/vcpkg
ENV PATH="/opt/vcpkg:${PATH}"
WORKDIR /app
COPY . .

ENV CC=gcc
ENV CXX=g++

RUN rm -rf build


RUN cmake -S . -B build -G Ninja \
    -DCMAKE_TOOLCHAIN_FILE=/opt/vcpkg/scripts/buildsystems/vcpkg.cmake \
    -DCMAKE_BUILD_TYPE=Release

RUN cmake --build build

# Runtime stage
FROM alpine:3.19 AS runtime
RUN apk add --no-cache \
    libstdc++ \
    libgcc

COPY --from=builder /app/build/calculator /usr/local/bin/calculator
EXPOSE 8080
CMD ["/usr/local/bin/calculator"]