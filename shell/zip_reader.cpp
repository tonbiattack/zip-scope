// Minimal ZIP central-directory reader.
#include "zip_reader.h"

#include <algorithm>
#include <cstring>

#include "decode.h"
#include "guids.h"

namespace zipscope {

namespace {

// Little-endian readers. The archive is untrusted, so every read is bounds
// checked by the caller before it gets here.
inline uint16_t Rd16(const BYTE* p) {
    return static_cast<uint16_t>(p[0] | (p[1] << 8));
}
inline uint32_t Rd32(const BYTE* p) {
    return static_cast<uint32_t>(p[0]) | (static_cast<uint32_t>(p[1]) << 8) |
           (static_cast<uint32_t>(p[2]) << 16) | (static_cast<uint32_t>(p[3]) << 24);
}
inline uint64_t Rd64(const BYTE* p) {
    return static_cast<uint64_t>(Rd32(p)) | (static_cast<uint64_t>(Rd32(p + 4)) << 32);
}

constexpr uint32_t kSigLocal = 0x04034b50;
constexpr uint32_t kSigCentral = 0x02014b50;
constexpr uint32_t kSigEnd = 0x06054b50;
constexpr uint32_t kSigEnd64 = 0x06064b50;
constexpr uint32_t kSigEnd64Locator = 0x07064b50;

// Entry is one central-directory record.
struct Entry {
    std::string name;
    uint16_t flags = 0;
    uint16_t method = 0;
    uint32_t crc = 0;
    uint64_t compressedSize = 0;
    uint64_t uncompressedSize = 0;
    uint64_t localHeaderOffset = 0;
};

// IsImageName reports whether the entry has a supported image extension.
bool IsImageName(const std::string& name) {
    size_t dot = name.find_last_of('.');
    if (dot == std::string::npos) return false;
    std::string ext = name.substr(dot);
    for (auto& c : ext) c = static_cast<char>(tolower(static_cast<unsigned char>(c)));
    return ext == ".jpg" || ext == ".jpeg" || ext == ".jpe" || ext == ".jfif" ||
           ext == ".png" || ext == ".gif" || ext == ".webp";
}

// NaturalLess compares two entry names the way the application does: digit runs
// compare numerically, case is folded, and the comparison is total.
bool NaturalLess(const std::string& a, const std::string& b) {
    size_t i = 0, j = 0;
    while (i < a.size() && j < b.size()) {
        unsigned char ca = static_cast<unsigned char>(a[i]);
        unsigned char cb = static_cast<unsigned char>(b[j]);
        if (isdigit(ca) && isdigit(cb)) {
            size_t si = i, sj = j;
            while (i < a.size() && isdigit(static_cast<unsigned char>(a[i]))) i++;
            while (j < b.size() && isdigit(static_cast<unsigned char>(b[j]))) j++;
            // Compare the digit runs by value, ignoring leading zeros.
            size_t ai = si, bj = sj;
            while (ai < i - 1 && a[ai] == '0') ai++;
            while (bj < j - 1 && b[bj] == '0') bj++;
            size_t la = i - ai, lb = j - bj;
            if (la != lb) return la < lb;
            int cmp = a.compare(ai, la, b, bj, lb);
            if (cmp != 0) return cmp < 0;
            continue;
        }
        unsigned char fa = static_cast<unsigned char>(tolower(ca));
        unsigned char fb = static_cast<unsigned char>(tolower(cb));
        if (fa != fb) return fa < fb;
        i++;
        j++;
    }
    if (i < a.size()) return false;
    if (j < b.size()) return true;
    return a < b;
}

}  // namespace

// InflateRaw decompresses a raw DEFLATE stream. It is implemented in
// inflate.cpp so the decoder lives in one place.
bool InflateRaw(const BYTE* src, size_t srcLen, size_t expected, std::vector<BYTE>* out);

bool ReadStreamToEnd(IStream* stream, size_t maxBytes, std::vector<BYTE>* out) {
    if (!stream || !out) return false;
    out->clear();

    // Ask for the total size when the stream reports one; otherwise grow.
    STATSTG stat = {};
    ULONGLONG hint = 0;
    if (SUCCEEDED(stream->Stat(&stat, STATFLAG_NONAME))) {
        hint = stat.cbSize.QuadPart;
        if (hint > maxBytes) return false;
    }

    const size_t chunk = 64 * 1024;
    std::vector<BYTE> buf(chunk);
    LARGE_INTEGER zero = {};
    stream->Seek(zero, STREAM_SEEK_SET, nullptr);

    for (;;) {
        ULONG read = 0;
        HRESULT hr = stream->Read(buf.data(), static_cast<ULONG>(buf.size()), &read);
        if (hr != S_OK && hr != S_FALSE) return false;
        if (read == 0) break;
        if (out->size() + read > maxBytes) return false;
        out->insert(out->end(), buf.begin(), buf.begin() + read);
        if (hr == S_FALSE) break;
    }
    return !out->empty();
}

bool ReadFirstImage(const std::vector<BYTE>& archive, size_t maxEntryBytes,
                    size_t maxPixels, std::vector<BYTE>* out) {
    if (!out) return false;
    out->clear();
    const size_t n = archive.size();
    if (n < 22) return false;
    const BYTE* base = archive.data();

    // Locate the end-of-central-directory record by scanning backwards. The
    // comment field is at most 64 KiB, so the search window is bounded.
    size_t eocd = SIZE_MAX;
    size_t window = std::min<size_t>(n, 22 + 0xFFFF);
    for (size_t i = n - 22 + 1; i-- > n - window;) {
        if (Rd32(base + i) == kSigEnd) {
            eocd = i;
            break;
        }
    }
    if (eocd == SIZE_MAX) return false;

    uint64_t entryCount = Rd16(base + eocd + 10);
    uint64_t cdSize = Rd32(base + eocd + 12);
    uint64_t cdOffset = Rd32(base + eocd + 16);

    // ZIP64: when any field is saturated, read the ZIP64 end record instead.
    if (entryCount == 0xFFFF || cdSize == 0xFFFFFFFFull || cdOffset == 0xFFFFFFFFull) {
        if (eocd >= 20 && Rd32(base + eocd - 20) == kSigEnd64Locator) {
            uint64_t z64 = Rd64(base + eocd - 20 + 8);
            if (z64 + 56 <= n && Rd32(base + z64) == kSigEnd64) {
                entryCount = Rd64(base + z64 + 32);
                cdSize = Rd64(base + z64 + 40);
                cdOffset = Rd64(base + z64 + 48);
            }
        }
    }

    if (cdOffset > n || cdSize > n - cdOffset) return false;

    // Walk the central directory and collect image entries.
    std::vector<Entry> images;
    size_t p = static_cast<size_t>(cdOffset);
    const size_t cdEnd = static_cast<size_t>(cdOffset + cdSize);
    for (uint64_t k = 0; k < entryCount && p + 46 <= cdEnd; k++) {
        if (Rd32(base + p) != kSigCentral) break;
        Entry e;
        e.flags = Rd16(base + p + 8);
        e.method = Rd16(base + p + 10);
        e.crc = Rd32(base + p + 16);
        e.compressedSize = Rd32(base + p + 20);
        e.uncompressedSize = Rd32(base + p + 24);
        uint16_t nameLen = Rd16(base + p + 28);
        uint16_t extraLen = Rd16(base + p + 30);
        uint16_t commentLen = Rd16(base + p + 32);
        e.localHeaderOffset = Rd32(base + p + 42);

        if (p + 46 + nameLen > cdEnd) break;
        e.name.assign(reinterpret_cast<const char*>(base + p + 46), nameLen);

        // ZIP64 extended information lives in the extra field.
        if (e.uncompressedSize == 0xFFFFFFFFull || e.compressedSize == 0xFFFFFFFFull ||
            e.localHeaderOffset == 0xFFFFFFFFull) {
            size_t x = p + 46 + nameLen;
            size_t xEnd = x + extraLen;
            while (x + 4 <= xEnd) {
                uint16_t id = Rd16(base + x);
                uint16_t sz = Rd16(base + x + 2);
                if (x + 4 + sz > xEnd) break;
                if (id == 0x0001) {
                    size_t q = x + 4;
                    if (e.uncompressedSize == 0xFFFFFFFFull && q + 8 <= x + 4 + sz) {
                        e.uncompressedSize = Rd64(base + q);
                        q += 8;
                    }
                    if (e.compressedSize == 0xFFFFFFFFull && q + 8 <= x + 4 + sz) {
                        e.compressedSize = Rd64(base + q);
                        q += 8;
                    }
                    if (e.localHeaderOffset == 0xFFFFFFFFull && q + 8 <= x + 4 + sz) {
                        e.localHeaderOffset = Rd64(base + q);
                    }
                }
                x += 4 + sz;
            }
        }

        if (IsImageName(e.name)) images.push_back(std::move(e));
        p += 46 + nameLen + extraLen + commentLen;
    }

    if (images.empty()) return false;

    // Natural order, with the central-directory order as a stable tie-break.
    std::stable_sort(images.begin(), images.end(),
                     [](const Entry& a, const Entry& b) { return NaturalLess(a.name, b.name); });

    // Take the first entry that is actually readable. A broken first image
    // should not hide a good second one.
    for (const Entry& e : images) {
        if (e.flags & 0x1) continue;                       // encrypted
        if (e.method != 0 && e.method != 8) continue;      // store or deflate
        if (e.uncompressedSize == 0 || e.uncompressedSize > maxEntryBytes) continue;
        if (e.localHeaderOffset + 30 > n) continue;
        if (Rd32(base + e.localHeaderOffset) != kSigLocal) continue;

        uint16_t nameLen = Rd16(base + e.localHeaderOffset + 26);
        uint16_t extraLen = Rd16(base + e.localHeaderOffset + 28);
        size_t dataStart = static_cast<size_t>(e.localHeaderOffset) + 30 + nameLen + extraLen;
        if (dataStart > n || e.compressedSize > n - dataStart) continue;

        const BYTE* src = base + dataStart;
        std::vector<BYTE> data;
        if (e.method == 0) {
            data.assign(src, src + e.compressedSize);
        } else if (!InflateRaw(src, static_cast<size_t>(e.compressedSize),
                               static_cast<size_t>(e.uncompressedSize), &data)) {
            continue;
        }
        if (data.size() != e.uncompressedSize) continue;

        // Verify the CRC so a corrupt entry is skipped rather than shown.
        uint32_t crc = 0;
        if (!ComputeCrc32(data.data(), data.size(), &crc) || crc != e.crc) continue;

        if (!WithinPixelLimit(data, maxPixels)) continue;

        *out = std::move(data);
        return true;
    }
    return false;
}

}  // namespace zipscope
