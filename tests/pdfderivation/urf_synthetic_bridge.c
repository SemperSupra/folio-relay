/*
 * Disposable research-only Apple URF/PWG raster bridge.
 * Input/output: one monochrome 8-bit PGM (P5) page, bounded A4 300dpi.
 * Leverages upstream libcups raster rather than inventing an Apple URF codec.
 * THIS IS NOT A PRODUCTION PARSER and is never installed in control images.
 */
#include <cups/raster.h>
#include <cups/pwg.h>
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define MAX_WIDTH 4096u
#define MAX_HEIGHT 4096u
#define MAX_PIXELS (12u * 1024u * 1024u)

static int bounded(const cups_page_header2_t *h) {
  return h->cupsWidth > 0 && h->cupsHeight > 0 &&
    h->cupsWidth <= MAX_WIDTH && h->cupsHeight <= MAX_HEIGHT &&
    (size_t)h->cupsWidth * h->cupsHeight <= MAX_PIXELS &&
    h->cupsBitsPerColor == 8 && h->cupsBitsPerPixel == 8 &&
    h->cupsBytesPerLine == h->cupsWidth &&
    h->cupsColorSpace == CUPS_CSPACE_SW;
}

static int encode(const char *source, const char *target) {
  FILE *in = fopen(source, "rb");
  cups_page_header2_t header;
  pwg_media_t *media = pwgMediaForPWG("iso_a4_210x297mm");
  cups_raster_t *out = NULL;
  unsigned width=0,height=0,maxval=0;
  unsigned char *line=NULL;
  int fd=-1, result=1, separator;
  if (!in || !media) { fprintf(stderr,"missing PGM/media\n"); goto done; }
  if (fscanf(in, "P5 %u %u %u", &width, &height, &maxval)!=3) {
    fprintf(stderr,"invalid PGM header\n"); goto done;
  }
  separator=fgetc(in);
  if (separator!= '\n' && separator!=' ') {
    fprintf(stderr,"invalid PGM data separator\n"); goto done;
  }
  if (maxval!=255 || !cupsRasterInitPWGHeader(
    &header,media,"sgray_8",300,300,"one-sided",NULL) ||
    !bounded(&header) || width!=header.cupsWidth || height!=header.cupsHeight) {
    fprintf(stderr,"PGM / Apple raster profile mismatch: input %u x %u\n",
            width,height);
    goto done;
  }
  header.cupsInteger[CUPS_RASTER_PWG_TotalPageCount]=1;
  fd=open(target,O_WRONLY|O_CREAT|O_EXCL,0600);
  if (fd<0) {perror("open URF");goto done;}
  out=cupsRasterOpen(fd,CUPS_RASTER_WRITE_APPLE);
  if (!out || !cupsRasterWriteHeader2(out,&header)) {
    fprintf(stderr,"unable to encode Apple raster header\n");goto done;
  }
  line=malloc(width);
  if (!line) goto done;
  for(unsigned y=0;y<height;y++){
    if (fread(line,1,width,in)!=width ||
        cupsRasterWritePixels(out,line,width)!=width) {
      fprintf(stderr,"incomplete PGM or URF raster row %u\n",y);goto done;
    }
  }
  if(fgetc(in)!=EOF) {fprintf(stderr,"trailing PGM data\n");goto done;}
  result=0;
done:
  free(line);
  if(out) cupsRasterClose(out);
  if(fd>=0) close(fd);
  if(in) fclose(in);
  if(result) unlink(target);
  return result;
}

static int decode(const char *source,const char *target) {
  int fd=open(source,O_RDONLY);
  cups_raster_t *in=NULL;
  cups_page_header2_t header, extra;
  FILE *out=NULL;
  unsigned char *line=NULL;
  int result=1;
  if(fd<0){perror("open URF");return result;}
  in=cupsRasterOpen(fd,CUPS_RASTER_READ);
  if(!in || !cupsRasterReadHeader2(in,&header) || !bounded(&header)){
    fprintf(stderr,"unsupported, missing or oversized grayscale Apple raster\n");
    goto done;
  }
  out=fopen(target,"wb");
  if(!out){perror("open PGM");goto done;}
  if(fprintf(out,"P5\n%u %u\n255\n",header.cupsWidth,header.cupsHeight)<0)
    goto done;
  line=malloc(header.cupsWidth);
  if(!line) goto done;
  for(unsigned y=0;y<header.cupsHeight;y++){
    if(cupsRasterReadPixels(in,line,header.cupsWidth)!=header.cupsWidth ||
       fwrite(line,1,header.cupsWidth,out)!=header.cupsWidth) {
      fprintf(stderr,"truncated URF data at row %u\n",y);goto done;
    }
  }
  if(cupsRasterReadHeader2(in,&extra)) {
    fprintf(stderr,"extra Apple raster page not admitted by 1-page oracle\n");
    goto done;
  }
  if(fflush(out)!=0)goto done;
  result=0;
done:
  free(line);
  if(out) fclose(out);
  if(in)cupsRasterClose(in);
  close(fd);
  if(result)unlink(target);
  return result;
}

int main(int argc,char **argv){
  if(argc!=4){fprintf(stderr,"usage: %s encode|decode INPUT OUTPUT\n",argv[0]);return 64;}
  if(!strcmp(argv[1],"encode"))return encode(argv[2],argv[3]);
  if(!strcmp(argv[1],"decode"))return decode(argv[2],argv[3]);
  fprintf(stderr,"unsupported operation\n");
  return 64;
}
