#include <cups/raster.h>
#include <cups/pwg.h>

#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char **argv)
{
  cups_page_header2_t header;
  cups_raster_t *raster;
  pwg_media_t *media;
  unsigned char *line;
  unsigned y;
  int fd;

  if (argc != 2)
  {
    fprintf(stderr, "usage: %s OUTPUT.urf\n", argv[0]);
    return 64;
  }

  media = pwgMediaForPWG("iso_a4_210x297mm");
  if (!media)
    return 1;

  if (!cupsRasterInitPWGHeader(&header, media, "sgray_8", 300, 300,
                               "one-sided", NULL))
    return 1;
  header.cupsInteger[CUPS_RASTER_PWG_TotalPageCount] = 1;

  fd = open(argv[1], O_CREAT | O_TRUNC | O_WRONLY, 0600);
  if (fd < 0)
    return 1;

  raster = cupsRasterOpen(fd, CUPS_RASTER_WRITE_APPLE);
  if (!raster)
    return 1;

  if (!cupsRasterWriteHeader2(raster, &header))
    return 1;

  line = malloc(header.cupsBytesPerLine);
  if (!line)
    return 1;
  memset(line, 0xff, header.cupsBytesPerLine);

  for (y = 0; y < header.cupsHeight; y ++)
  {
    if (cupsRasterWritePixels(raster, line, header.cupsBytesPerLine) !=
        header.cupsBytesPerLine)
      return 1;
  }

  free(line);
  cupsRasterClose(raster);
  return close(fd) == 0 ? 0 : 1;
}
