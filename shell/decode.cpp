// CRC-32 and WIC-based image decoding.
#include "decode.h"

#include <wincodec.h>
#include <wrl/client.h>

#include <algorithm>
#include <cstring>

#include "guids.h"

using Microsoft::WRL::ComPtr;

namespace zipscope {

namespace {

// CRC-32 table, built once on first use.
struct Crc32Table {
    uint32_t v[256];
    Crc32Table() {
        for (uint32_t i = 0; i < 256; i++) {
            uint32_t c = i;
            for (int k = 0; k < 8; k++) {
                c = (c & 1) ? (0xEDB88320u ^ (c >> 1)) : (c >> 1);
            }
            v[i] = c;
        }
    }
};

const Crc32Table& Table() {
    static Crc32Table t;
    return t;
}

}  // namespace

bool ComputeCrc32(const BYTE* data, size_t len, uint32_t* out) {
    if (!out) return false;
    const Crc32Table& t = Table();
    uint32_t c = 0xFFFFFFFFu;
    for (size_t i = 0; i < len; i++) {
        c = t.v[(c ^ data[i]) & 0xFF] ^ (c >> 8);
    }
    *out = c ^ 0xFFFFFFFFu;
    return true;
}

namespace {

// InflateRaw is implemented in inflate.cpp; this forward declaration keeps the
// DEFLATE decoder in one place.
bool InflateRaw(const BYTE* src, size_t srcLen, size_t expected, std::vector<BYTE>* out);

}  // namespace

bool WithinPixelLimit(const std::vector<BYTE>& image, size_t maxPixels) {
    if (image.empty()) return false;

    ComPtr<IWICImagingFactory> factory;
    if (FAILED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                IID_PPV_ARGS(&factory)))) {
        return false;
    }

    ComPtr<IWICStream> stream;
    if (FAILED(factory->CreateStream(&stream))) return false;
    if (FAILED(stream->InitializeFromMemory(const_cast<BYTE*>(image.data()),
                                            static_cast<DWORD>(image.size())))) {
        return false;
    }

    ComPtr<IWICBitmapDecoder> decoder;
    if (FAILED(factory->CreateDecoderFromStream(stream.Get(), nullptr,
                                                WICDecodeMetadataCacheOnDemand,
                                                &decoder))) {
        return false;
    }

    ComPtr<IWICBitmapFrameDecode> frame;
    if (FAILED(decoder->GetFrame(0, &frame))) return false;

    UINT w = 0, h = 0;
    if (FAILED(frame->GetSize(&w, &h))) return false;
    if (w == 0 || h == 0) return false;
    return static_cast<uint64_t>(w) * static_cast<uint64_t>(h) <= maxPixels;
}

bool DecodeToHBITMAP(const std::vector<BYTE>& image, UINT maxSide, HBITMAP* out) {
    if (!out) return false;
    *out = nullptr;
    if (image.empty()) return false;
    if (maxSide == 0) maxSide = 256;
    if (maxSide > kMaxThumbnailSide) maxSide = kMaxThumbnailSide;

    ComPtr<IWICImagingFactory> factory;
    if (FAILED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                IID_PPV_ARGS(&factory)))) {
        return false;
    }

    ComPtr<IWICStream> stream;
    if (FAILED(factory->CreateStream(&stream))) return false;
    if (FAILED(stream->InitializeFromMemory(const_cast<BYTE*>(image.data()),
                                            static_cast<DWORD>(image.size())))) {
        return false;
    }

    ComPtr<IWICBitmapDecoder> decoder;
    if (FAILED(factory->CreateDecoderFromStream(stream.Get(), nullptr,
                                                WICDecodeMetadataCacheOnDemand,
                                                &decoder))) {
        return false;
    }

    ComPtr<IWICBitmapFrameDecode> frame;
    if (FAILED(decoder->GetFrame(0, &frame))) return false;

    UINT srcW = 0, srcH = 0;
    if (FAILED(frame->GetSize(&srcW, &srcH)) || srcW == 0 || srcH == 0) return false;

    // Scale so the longest side fits maxSide; never upscale.
    UINT dstW = srcW, dstH = srcH;
    if (srcW > maxSide || srcH > maxSide) {
        if (srcW >= srcH) {
            dstW = maxSide;
            dstH = std::max<UINT>(1, static_cast<UINT>(
                static_cast<uint64_t>(srcH) * maxSide / srcW));
        } else {
            dstH = maxSide;
            dstW = std::max<UINT>(1, static_cast<UINT>(
                static_cast<uint64_t>(srcW) * maxSide / srcH));
        }
    }

    ComPtr<IWICFormatConverter> converter;
    if (FAILED(factory->CreateFormatConverter(&converter))) return false;
    if (FAILED(converter->Initialize(frame.Get(), GUID_WICPixelFormat32bppPBGRA,
                                     WICBitmapDitherTypeNone, nullptr, 0.0,
                                     WICBitmapPaletteTypeCustom))) {
        return false;
    }

    ComPtr<IWICBitmapScaler> scaler;
    if (FAILED(factory->CreateBitmapScaler(&scaler))) return false;
    if (FAILED(scaler->Initialize(converter.Get(), dstW, dstH, WICBitmapInterpolationModeFant))) {
        return false;
    }

    // Build a 32-bit DIB and copy the pixels into it.
    BITMAPINFO bmi = {};
    bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bmi.bmiHeader.biWidth = static_cast<LONG>(dstW);
    bmi.bmiHeader.biHeight = -static_cast<LONG>(dstH);  // top-down
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB;

    void* bits = nullptr;
    HBITMAP bmp = CreateDIBSection(nullptr, &bmi, DIB_RGB_COLORS, &bits, nullptr, 0);
    if (!bmp || !bits) {
        if (bmp) DeleteObject(bmp);
        return false;
    }

    const UINT stride = dstW * 4;
    const UINT bufSize = stride * dstH;
    if (FAILED(scaler->CopyPixels(nullptr, stride, bufSize, static_cast<BYTE*>(bits)))) {
        DeleteObject(bmp);
        return false;
    }

    *out = bmp;
    return true;
}

}  // namespace zipscope
