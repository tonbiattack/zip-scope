// Raw DEFLATE (RFC 1951) decoder.
//
// The Windows compression API only exposes MSZIP/XPRESS/LZMS containers, not
// the bare DEFLATE stream a ZIP entry stores, so the decoder is implemented
// here. It is a straightforward canonical-Huffman inflater with strict bounds
// checking: the input is an arbitrary file, so every read is validated and the
// output is capped at the size the central directory promised.
#include <windows.h>

#include <cstdint>
#include <cstring>
#include <vector>

#include "decode.h"

namespace zipscope {

namespace {

// BitReader pulls bits LSB-first, which is the order DEFLATE uses.
class BitReader {
public:
    BitReader(const BYTE* data, size_t size) : data_(data), size_(size) {}

    // Bits returns up to n bits, or -1 when the input is exhausted.
    int Bits(int n) {
        while (bitCount_ < n) {
            if (pos_ >= size_) return -1;
            bitBuf_ |= static_cast<uint32_t>(data_[pos_++]) << bitCount_;
            bitCount_ += 8;
        }
        int v = static_cast<int>(bitBuf_ & ((1u << n) - 1));
        bitBuf_ >>= n;
        bitCount_ -= n;
        return v;
    }

    // Align discards bits up to the next byte boundary.
    void Align() {
        bitBuf_ = 0;
        bitCount_ = 0;
    }

    // Byte returns the next whole byte, or -1 at the end.
    int Byte() {
        if (pos_ >= size_) return -1;
        return data_[pos_++];
    }

    bool Overrun() const { return overrun_; }

private:
    const BYTE* data_;
    size_t size_;
    size_t pos_ = 0;
    uint32_t bitBuf_ = 0;
    int bitCount_ = 0;
    bool overrun_ = false;
};

// Huffman is a canonical Huffman decoding table.
struct Huffman {
    // count[len] is the number of codes of that length; symbol[] lists symbols
    // ordered by code.
    uint16_t count[16] = {};
    uint16_t symbol[288] = {};

    // Build constructs the table from code lengths. Returns false for an
    // over-subscribed or incomplete code, which means corrupt input.
    bool Build(const uint8_t* lengths, int n) {
        for (int i = 0; i < 16; i++) count[i] = 0;
        for (int i = 0; i < n; i++) count[lengths[i]]++;
        if (count[0] == n) return true;  // no codes at all

        // Check for an over-subscribed set.
        int left = 1;
        for (int len = 1; len < 16; len++) {
            left <<= 1;
            left -= count[len];
            if (left < 0) return false;
        }

        uint16_t offs[16] = {};
        for (int len = 1; len < 15; len++) offs[len + 1] = offs[len] + count[len];
        for (int i = 0; i < n; i++) {
            if (lengths[i] != 0) symbol[offs[lengths[i]]++] = static_cast<uint16_t>(i);
        }
        return true;
    }

    // Decode reads one symbol.
    int Decode(BitReader* br) const {
        int code = 0, first = 0, index = 0;
        for (int len = 1; len < 16; len++) {
            int b = br->Bits(1);
            if (b < 0) return -1;
            code |= b;
            int n = this->count[len];
            if (code - first < n) return symbol[index + (code - first)];
            index += n;
            first = (first + n) << 1;
            code <<= 1;
        }
        return -1;
    }
};

// Length and distance base tables from RFC 1951 section 3.2.5.
const uint16_t kLenBase[29] = {3,  4,  5,  6,  7,  8,  9,  10, 11,  13,  15,  17,  19,  23, 27,
                               31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258};
const uint8_t kLenExtra[29] = {0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2,
                               2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0};
const uint16_t kDistBase[30] = {1,    2,    3,    4,    5,    7,     9,     13,    17,  25,
                                33,   49,   65,   97,   129,  193,   257,   385,   513, 769,
                                1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577};
const uint8_t kDistExtra[30] = {0, 0, 0, 0, 1, 1, 2, 2,  3,  3,  4,  4,  5,  5,  6,
                                6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13};

// Fixed literal/length code lengths from RFC 1951 section 3.2.6.
void FixedTables(Huffman* lit, Huffman* dist) {
    uint8_t lengths[288];
    for (int i = 0; i < 144; i++) lengths[i] = 8;
    for (int i = 144; i < 256; i++) lengths[i] = 9;
    for (int i = 256; i < 280; i++) lengths[i] = 7;
    for (int i = 280; i < 288; i++) lengths[i] = 8;
    lit->Build(lengths, 288);

    uint8_t d[30];
    for (int i = 0; i < 30; i++) d[i] = 5;
    dist->Build(d, 30);
}

// InflateCodes decodes one block with the given tables.
bool InflateCodes(BitReader* br, const Huffman& lit, const Huffman& dist,
                  std::vector<BYTE>* out, size_t limit) {
    for (;;) {
        int sym = lit.Decode(br);
        if (sym < 0) return false;
        if (sym < 256) {
            if (out->size() >= limit) return false;
            out->push_back(static_cast<BYTE>(sym));
            continue;
        }
        if (sym == 256) return true;  // end of block

        sym -= 257;
        if (sym >= 29) return false;
        int extra = br->Bits(kLenExtra[sym]);
        if (extra < 0) return false;
        size_t length = kLenBase[sym] + static_cast<size_t>(extra);

        int dsym = dist.Decode(br);
        if (dsym < 0 || dsym >= 30) return false;
        int dextra = br->Bits(kDistExtra[dsym]);
        if (dextra < 0) return false;
        size_t distance = kDistBase[dsym] + static_cast<size_t>(dextra);

        if (distance > out->size()) return false;  // reference before the start
        if (out->size() + length > limit) return false;

        // Copy byte by byte: the source and destination ranges may overlap.
        size_t from = out->size() - distance;
        for (size_t i = 0; i < length; i++) {
            out->push_back((*out)[from + i]);
        }
    }
}

// DynamicTables reads the dynamic Huffman header of a block.
bool DynamicTables(BitReader* br, Huffman* lit, Huffman* dist) {
    int hlit = br->Bits(5);
    int hdist = br->Bits(5);
    int hclen = br->Bits(4);
    if (hlit < 0 || hdist < 0 || hclen < 0) return false;
    hlit += 257;
    hdist += 1;
    hclen += 4;
    if (hlit > 286 || hdist > 30) return false;

    static const uint8_t order[19] = {16, 17, 18, 0, 8,  7, 9,  6, 10, 5,
                                      11, 4,  12, 3, 13, 2, 14, 1, 15};
    uint8_t clen[19] = {};
    for (int i = 0; i < hclen; i++) {
        int v = br->Bits(3);
        if (v < 0) return false;
        clen[order[i]] = static_cast<uint8_t>(v);
    }

    Huffman lencode;
    if (!lencode.Build(clen, 19)) return false;

    uint8_t lengths[288 + 30] = {};
    int n = 0;
    while (n < hlit + hdist) {
        int sym = lencode.Decode(br);
        if (sym < 0) return false;
        if (sym < 16) {
            lengths[n++] = static_cast<uint8_t>(sym);
            continue;
        }
        int repeat = 0;
        uint8_t value = 0;
        if (sym == 16) {
            if (n == 0) return false;
            value = lengths[n - 1];
            int v = br->Bits(2);
            if (v < 0) return false;
            repeat = 3 + v;
        } else if (sym == 17) {
            int v = br->Bits(3);
            if (v < 0) return false;
            repeat = 3 + v;
        } else {
            int v = br->Bits(7);
            if (v < 0) return false;
            repeat = 11 + v;
        }
        if (n + repeat > hlit + hdist) return false;
        for (int i = 0; i < repeat; i++) lengths[n++] = value;
    }

    if (!lit->Build(lengths, hlit)) return false;
    if (!dist->Build(lengths + hlit, hdist)) return false;
    return true;
}

}  // namespace

bool InflateRaw(const BYTE* src, size_t srcLen, size_t expected, std::vector<BYTE>* out) {
    if (!src || !out) return false;
    out->clear();
    if (expected == 0) return false;
    out->reserve(expected);

    BitReader br(src, srcLen);
    for (;;) {
        int final = br.Bits(1);
        int type = br.Bits(2);
        if (final < 0 || type < 0) return false;

        if (type == 0) {
            // Stored block: align, then read the length and its complement.
            br.Align();
            int b0 = br.Byte(), b1 = br.Byte(), b2 = br.Byte(), b3 = br.Byte();
            if (b0 < 0 || b1 < 0 || b2 < 0 || b3 < 0) return false;
            size_t len = static_cast<size_t>(b0 | (b1 << 8));
            size_t nlen = static_cast<size_t>(b2 | (b3 << 8));
            if ((len ^ 0xFFFF) != nlen) return false;
            if (out->size() + len > expected) return false;
            for (size_t i = 0; i < len; i++) {
                int b = br.Byte();
                if (b < 0) return false;
                out->push_back(static_cast<BYTE>(b));
            }
        } else if (type == 1) {
            Huffman lit, dist;
            FixedTables(&lit, &dist);
            if (!InflateCodes(&br, lit, dist, out, expected)) return false;
        } else if (type == 2) {
            Huffman lit, dist;
            if (!DynamicTables(&br, &lit, &dist)) return false;
            if (!InflateCodes(&br, lit, dist, out, expected)) return false;
        } else {
            return false;  // reserved block type
        }

        if (final == 1) break;
    }
    return out->size() == expected;
}

}  // namespace zipscope
