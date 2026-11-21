// Image decoding and ZIP decompression helpers.
#pragma once

#include <windows.h>

#include <cstdint>
#include <vector>

namespace zipscope {

// ComputeCrc32 calculates the standard ZIP CRC-32 of a buffer.
bool ComputeCrc32(const BYTE* data, size_t len, uint32_t* out);

// WithinPixelLimit reads the image header and reports whether width*height is
// within the limit. It never decodes the full image.
bool WithinPixelLimit(const std::vector<BYTE>& image, size_t maxPixels);

// DecodeToHBITMAP decodes an image and scales it so its longest side is at most
// maxSide, returning a 32-bit ARGB HBITMAP suitable for Explorer.
bool DecodeToHBITMAP(const std::vector<BYTE>& image, UINT maxSide, HBITMAP* out);

}  // namespace zipscope
