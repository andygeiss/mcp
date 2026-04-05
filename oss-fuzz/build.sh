#!/bin/bash -eu

compile-native-go-fuzzer github.com/andygeiss/mcp/internal/protocol Fuzz_Decoder_With_ArbitraryInput fuzz_decoder_with_arbitrary_input
