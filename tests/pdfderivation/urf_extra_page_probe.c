/*
 * Source-only two-page Apple URF negative fixture. Reuses upstream libcups.
 * Generates a real 2-page URF stream from one admitted bounded grayscale
 * page. It is NOT a production decoder or converter.
 */
#include <cups/raster.h>
#include <cups/pwg.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

#define MAX_PAGE_BYTES (12u * 1024u * 1024u)

int main(int argc, char **argv) {
  if (argc != 3) {
    fprintf(stderr, "usage: %s ONE_PAGE.urf TWO_PAGE.urf\n", argv[0]);
    return 64;
  }
  int fd = -1, output_fd = -1, result = 1;
  cups_raster_t *input = NULL, *output = NULL;
  cups_page_header2_t h;
  unsigned char *bytes = NULL;
  fd = open(argv[1], O_RDONLY);
  if (fd < 0) goto done;
  input = cupsRasterOpen(fd, CUPS_RASTER_READ);
  if (!input || !cupsRasterReadHeader2(input, &h) ||
      h.cupsWidth < 1 || h.cupsHeight < 1 ||
      h.cupsBytesPerLine != h.cupsWidth ||
      h.cupsBitsPerPixel != 8 || h.cupsBitsPerColor != 8 ||
      h.cupsColorSpace != CUPS_CSPACE_SW ||
      h.cupsHeight > MAX_PAGE_BYTES / h.cupsBytesPerLine) {
    fprintf(stderr, "input profile not admitted\n");
    goto done;
  }
  const size_t page_bytes = (size_t)h.cupsHeight * h.cupsBytesPerLine;
  bytes = malloc(page_bytes);
  if (!bytes) goto done;
  for (unsigned y = 0; y < h.cupsHeight; y++) {
    size_t off = (size_t)y * h.cupsBytesPerLine;
    if (cupsRasterReadPixels(input, bytes + off, h.cupsBytesPerLine) != h.cupsBytesPerLine) {
      fprintf(stderr, "source is incomplete\n");
      goto done;
    }
  }
  cups_page_header2_t extra;
  if (cupsRasterReadHeader2(input, &extra)) {
    fprintf(stderr, "source must have one page\n");
    goto done;
  }
  output_fd = open(argv[2], O_WRONLY | O_CREAT | O_EXCL, 0600);
  if (output_fd < 0) goto done;
  output = cupsRasterOpen(output_fd, CUPS_RASTER_WRITE_APPLE);
  if (!output) goto done;
  h.cupsInteger[CUPS_RASTER_PWG_TotalPageCount] = 2;
  for (int page = 0; page < 2; page++) {
    if (!cupsRasterWriteHeader2(output, &h)) goto done;
    for (unsigned y = 0; y < h.cupsHeight; y++) {
      size_t off = (size_t)y * h.cupsBytesPerLine;
      if (cupsRasterWritePixels(output, bytes + off, h.cupsBytesPerLine) != h.cupsBytesPerLine)
        goto done;
    }
  }
  result = 0;
done:
  free(bytes);
  if (output) cupsRasterClose(output);
  if (input) cupsRasterClose(input);
  if (output_fd >= 0) close(output_fd);
  if (fd >= 0) close(fd);
  if (result) unlink(argv[2]);
  return result;
}
