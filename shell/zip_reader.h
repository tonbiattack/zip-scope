// Minimal, defensive ZIP reader for the shell extension.
//
// This is intentionally not a general-purpose ZIP library. It reads only what
// the extensions need: the central directory, the image entries in natural
// order, and the bytes of the entries that are actually displayed. Everything is
// bounds-checked because the input is an arbitrary file from the user's disk.
//
// Two consumers use it:
//
//   - the thumbnail provider, which wants just the first image;
//   - the preview handler, which wants the whole list and reads entries lazily
//     as the user scrolls.
//
// Both share one index so the parsing rules (natural order, limits, rejecting
// encrypted entries) cannot drift apart.
#pragma once

#include <windows.h>

#include <cstdint>
#include <string>
#include <vector>

namespace zipscope {

// Entry is one image in an archive's central directory.
struct ZipEntry {
    std::string name;
    uint16_t flags = 0;
    uint16_t method = 0;
    uint32_t crc = 0;
    uint64_t compressedSize = 0;
    uint64_t uncompressedSize = 0;
    uint64_t localHeaderOffset = 0;

    // Decodable reports whether the entry can be extracted at all. An encrypted
    // or unsupported entry is listed so the UI can show why it is blank.
    bool Decodable() const {
        if (flags & 0x1) return false;                  // encrypted
        if (method != 0 && method != 8) return false;   // store or deflate only
        return uncompressedSize > 0;
    }
};

// Limits bounds the work one archive may cause. The values are the shell
// extension's own; they are deliberately tighter than the desktop app's because
// Explorer hosts this in-process.
struct ZipLimits {
    size_t maxEntries = 10000;
    size_t maxEntryBytes = 128ull * 1024 * 1024;
    size_t maxPixels = 40ull * 1000 * 1000;
};

// ZipArchive is an indexed archive. The bytes are held in memory so entries can
// be extracted on demand without reopening the file; Explorer hands us a stream,
// not a path, and re-reading the stream for every thumbnail would be far slower.
class ZipArchive {
public:
    // Index parses the central directory and records the image entries in
    // natural order. It does not extract anything.
    bool Index(const std::vector<BYTE>& bytes, const ZipLimits& limits);

    // Entries returns the image entries in natural order.
    const std::vector<ZipEntry>& Entries() const { return entries_; }

    // Count returns the number of image entries.
    size_t Count() const { return entries_.size(); }

    // Truncated reports whether the entry limit cut the listing short, so the UI
    // can say "showing the first N of more".
    bool Truncated() const { return truncated_; }

    // Extract returns the uncompressed bytes of entry i, verifying the CRC and
    // enforcing the byte and pixel limits. It returns false for a broken entry
    // rather than failing the whole archive.
    bool Extract(size_t i, std::vector<BYTE>* out) const;

private:
    const BYTE* base_ = nullptr;
    size_t size_ = 0;
    std::vector<ZipEntry> entries_;
    ZipLimits limits_;
    bool truncated_ = false;
};

// ReadStreamToEnd copies an IStream into a byte vector, refusing anything
// larger than maxBytes.
bool ReadStreamToEnd(IStream* stream, size_t maxBytes, std::vector<BYTE>* out);

}  // namespace zipscope
