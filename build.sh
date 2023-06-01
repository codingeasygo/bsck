#!/bin/bash

cargo lipo --release
mkdir -p $1/include/ $1/lib/
cp bind/src/router.h $1/include/
cp target/universal/release/librouter.a $1/lib/