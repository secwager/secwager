FROM alpine:3.19 AS builder

RUN apk add --no-cache \
    build-base cmake ninja git curl zip unzip \
    linux-headers pkgconfig gcompat

ENV VCPKG_ROOT=/opt/vcpkg
RUN git clone https://github.com/microsoft/vcpkg.git $VCPKG_ROOT && \
    $VCPKG_ROOT/bootstrap-vcpkg.sh -disableMetrics

COPY . /src
WORKDIR /src

RUN cmake --preset default -DCMAKE_BUILD_TYPE=Release -B build && \
    cmake --build build --parallel

FROM builder AS test
RUN ctest --test-dir /src/build --output-on-failure
