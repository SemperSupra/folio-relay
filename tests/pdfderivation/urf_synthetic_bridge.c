#define _POSIX_C_SOURCE 200809L
/*
 * Disposable research-only Apple URF/PWG raster bridge.
 * Input/output: one monochrome 8-bit PGM (P5) page, bounded A4 300dpi.
 * Leverages upstream libcups raster rather than inventing an Apple URF codec.
 * THIS IS NOT A PRODUCTION PARSER and is never installed in control images.
 */
#include <cups/raster.h>
#include <cups/pwg.h>
#include <errno.h>
#include <sys/stat.h>
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

// SOURCE-ONLY improvement: an output parent must be a non-symlink directory
// reached through descriptor-relative, no-follow traversal and privately
// writable by this UID. This does not establish a protected process identity
// or defend against another same-UID actor inside that private directory.
static int private_output_parent(const char *target, const char **leaf) {
  if(!target || !*target) return -1;
  const char *slash=strrchr(target,'/');
  const char *base=slash?slash+1:target;
  if(!*base || !strcmp(base,".") || !strcmp(base,"..")) return -1;
  int dirfd=open(target[0]=='/'?"/":".",O_RDONLY|O_DIRECTORY|O_NOFOLLOW);
  if(dirfd<0) return -1;
  if(slash) {
    char *path=strndup(target,(size_t)(slash-target));
    if(!path){close(dirfd);return -1;}
    char *save=NULL;
    char *part=strtok_r(path,"/",&save);
    while(part) {
      if(!strcmp(part,"..")){free(path);close(dirfd);return -1;}
      if(strcmp(part,".")) {
        int next=openat(dirfd,part,O_RDONLY|O_DIRECTORY|O_NOFOLLOW);
        if(next<0){free(path);close(dirfd);return -1;}
        close(dirfd);
        dirfd=next;
      }
      part=strtok_r(NULL,"/",&save);
    }
    free(path);
  }
  struct stat st;
  if(fstat(dirfd,&st)!=0 || !S_ISDIR(st.st_mode) ||
     st.st_uid!=geteuid() || (st.st_mode&(S_IRWXG|S_IRWXO))!=0 ||
     (st.st_mode&(S_IWUSR|S_IXUSR))!=(S_IWUSR|S_IXUSR)){
    close(dirfd);
    return -1;
  }
  *leaf=base;
  return dirfd;
}

// Unlink by pinned directory descriptor only if the pathname still refers to
// the inode we created. This check is not an atomic compare-and-unlink and
// cannot secure a hostile same-UID collaborator; production requires an
// independent private worker/staging directory and publication protocol.
static void discard_partial_if_owned(int parentfd,const char *leaf,
                                     const struct stat *created_stat,int created){
  if(!created || parentfd<0 || !leaf)return;
  struct stat current;
  if(fstatat(parentfd,leaf,&current,AT_SYMLINK_NOFOLLOW)==0 &&
     current.st_dev==created_stat->st_dev &&
     current.st_ino==created_stat->st_ino){
    (void)unlinkat(parentfd,leaf,0);
  }
}

static int encode(const char *source, const char *target) {
  FILE *in = NULL;
  int sourcefd = -1;
  struct stat sourceinfo;
  cups_page_header2_t header;
  pwg_media_t *media = pwgMediaForPWG("iso_a4_210x297mm");
  cups_raster_t *out = NULL;
  unsigned width=0,height=0,maxval=0;
  unsigned char *line=NULL;
  int fd=-1, parentfd=-1, created=0, result=1, separator;
  const char *leaf=NULL;
  struct stat owned_output={0};
  // The synthetic derivative helper must not follow a final source symlink
  // or wait indefinitely on FIFO/device input before it can inspect type.
  sourcefd=open(source,O_RDONLY|O_NOFOLLOW|O_NONBLOCK);
  if (sourcefd<0 || fstat(sourcefd,&sourceinfo)!=0 ||
      !S_ISREG(sourceinfo.st_mode)) {
    fprintf(stderr,"encode source must be a regular non-symlink file\\n");
    goto done;
  }
  in=fdopen(sourcefd,"rb");
  if (!in || !media) { fprintf(stderr,"missing PGM/media\\n"); goto done; }
  sourcefd=-1;
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
    fprintf(stderr,
      "PGM / Apple raster profile mismatch: input %u x %u; generated %u x %u; "
      "bits=%u/%u bpl=%u cspace=%d bounded=%d\\n",
      width,height,header.cupsWidth,header.cupsHeight,
      header.cupsBitsPerColor,header.cupsBitsPerPixel,
      header.cupsBytesPerLine,header.cupsColorSpace,bounded(&header));
    goto done;
  }
  header.cupsInteger[CUPS_RASTER_PWG_TotalPageCount]=1;
  parentfd=private_output_parent(target,&leaf);
  if(parentfd<0){fprintf(stderr,"unsafe encode output directory\n");goto done;}
  fd=openat(parentfd,leaf,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0600);
  if(fd<0){perror("exclusive URF output");goto done;}
  if(fstat(fd,&owned_output)!=0){perror("stat owned URF");goto done;}
  created=1;
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
  if(sourcefd>=0) close(sourcefd);
  if(result)discard_partial_if_owned(parentfd,leaf,&owned_output,created);
  if(parentfd>=0)close(parentfd);
  return result;
}

static int decode(const char *source,const char *target) {
  int fd=open(source,O_RDONLY|O_NOFOLLOW|O_NONBLOCK);
  struct stat sourceinfo;
  cups_raster_t *in=NULL;
  cups_page_header2_t header, extra;
  FILE *out=NULL;
  unsigned char *line=NULL;
  int outfd=-1, parentfd=-1, created=0, result=1;
  const char *leaf=NULL;
  struct stat owned_output={0};
  if(fd<0){perror("open URF");return result;}
  if(fstat(fd,&sourceinfo)!=0 || !S_ISREG(sourceinfo.st_mode)) {
    fprintf(stderr,"decode source must be a regular non-symlink file\\n");
    close(fd);
    return result;
  }
  // The upstream reader accepts some alternate header variants. This source
  // preview profile admits ONLY exact seven-byte Apple "UNIRAST" magic.
  // Inspect the same open fd later passed to libcups (no pathname reopen).
  unsigned char signature[7];
  if(read(fd,signature,sizeof(signature))!=(ssize_t)sizeof(signature) ||
      memcmp(signature,"UNIRAST",sizeof(signature))!=0 ||
      lseek(fd,0,SEEK_SET)!=0) {
    fprintf(stderr,"invalid Apple raster signature\n");
    goto done;
  }
  in=cupsRasterOpen(fd,CUPS_RASTER_READ);
  if(!in || !cupsRasterReadHeader2(in,&header) || !bounded(&header)){
    fprintf(stderr,"unsupported, missing or oversized grayscale Apple raster\n");
    goto done;
  }
  // Source-only output must be newly owned: no canonical clobber, no
  // existing file overwrite, and no symlink following. Still NOT a
  // production-safe cross-directory/concurrent-writer protocol.
  parentfd=private_output_parent(target,&leaf);
  if(parentfd<0){fprintf(stderr,"unsafe decode output directory\n");goto done;}
  outfd=openat(parentfd,leaf,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0600);
  if(outfd<0){perror("exclusive preview output");goto done;}
  if(fstat(outfd,&owned_output)!=0){perror("stat owned preview");goto done;}
  created=1;
  out=fdopen(outfd,"wb");
  if(!out){perror("fdopen preview output");goto done;}
  outfd=-1;
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
  if(out && fclose(out)!=0) result=1;
  if(outfd>=0)close(outfd);
  if(in)cupsRasterClose(in);
  close(fd);
  if(result)discard_partial_if_owned(parentfd,leaf,&owned_output,created);
  if(parentfd>=0)close(parentfd);
  return result;
}

int main(int argc,char **argv){
  if(argc!=4){fprintf(stderr,"usage: %s encode|decode INPUT OUTPUT\n",argv[0]);return 64;}
  if(!strcmp(argv[1],"encode"))return encode(argv[2],argv[3]);
  if(!strcmp(argv[1],"decode"))return decode(argv[2],argv[3]);
  fprintf(stderr,"unsupported operation\n");
  return 64;
}
