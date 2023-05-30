#!/bin/bash

cargo lipo --release
cp bind/src/router.h target/universal/release/