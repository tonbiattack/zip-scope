// Minimal, defensive ZIP reader for the shell extension.
//
// This is intentionally not a general-purpose ZIP library. It reads only what
// the thumbnail provider needs: the central directory, the first image entry in
// natural order, and that entry's bytes. Everything is bounds-checked because
// the input is an arbitrary file from the user's disk.
#pragma once

#include <windows.h>

#include <cstdint>
#include <string>
#include <vector>

namespace zipscope {

// ReadStreamToEnd copies an IStream into a byte vector, refusing anything
// larger than maxBytes.
bool ReadStreamToEnd(IStream* stream, size_t maxBytes, std::vector<BYTE>* out);

// ReadFirstImage finds the first image entry in natural order and returns its
// uncompressed bytes. It enforces the entry-size and pixel limits.
bool ReadFirstImage(const std::vector<BYTE>& archive, size_t maxEntryBytes,
                    size_t maxPixels, std::vector<BYTE>* out);

}  // namespace zipscope
