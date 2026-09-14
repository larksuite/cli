# Image dimensions

`Decode(io.ReaderAt)` reads width, height and format without changing the source
position. Callers upload the original bytes; they do not decode, resize or
re-encode pixels. `Config` intentionally contains no color model. A successful
result establishes dimensions, not the validity of all pixel data.

PNG, JPEG and GIF use the registered standard-library configuration readers.
TIFF, BMP and WebP use independently implemented metadata readers with no
external image codec dependency:

- TIFF: classic little/big-endian TIFF, scalar SHORT/LONG width and height in
  the first IFD. Other tag payloads and subsequent IFDs are never followed.
  BigTIFF is unsupported. The 16-bit entry count bounds traversal to 65535
  entries; buffers do not grow with file offsets or tag payload sizes.
- BMP: Windows 40-, 108- and 124-byte DIB headers, including top-down images,
  supported palette depths, and the existing default bitfield masks. Palette
  reads are bounded to 1024 bytes. Pixel data is not read.
- WebP: VP8, VP8L and VP8X dimension headers, with RIFF bounds and chunk padding
  included in offset calculations. Unknown chunks are skipped using random
  access. Traversal stops after 4096 chunks; supporting more requires an explicit
  resource-limit review and a real compatibility case. VP8X dimensions describe
  the canvas, including for animated images.

These readers validate the metadata needed for dimensions. They deliberately do
not parse TIFF compression/color metadata, WebP frame bodies or embedded EXIF/ICC
profiles. Do not use them as complete-file validators. If pixel processing is
introduced, select and review a separate codec at that boundary instead of
extending these metadata readers into decoders.

Format references:
- https://www.adobe.io/content/dam/udp/en/open/standards/tiff/TIFF6.pdf
- https://learn.microsoft.com/en-us/windows/win32/api/wingdi/ns-wingdi-bitmapinfoheader
- https://developers.google.com/speed/webp/docs/riff_container
