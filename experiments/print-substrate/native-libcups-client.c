// Direct native libcups Print-Job probe for FolioRelay.
//
// This deliberately bypasses the local CUPS scheduler and cups-filters. It
// exercises the host's native libcups IPP transport against an explicit
// driverless printer URI so client conversion bugs cannot be mistaken for
// FolioRelay substrate failures.
#include <cups/cups.h>
#include <cups/http.h>
#include <cups/ipp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char *argv[])
{
  char scheme[32], userpass[256], host[256], resource[1024];
  int port;
  http_t *http;
  ipp_t *request, *response;
  ipp_attribute_t *job_id;
  http_encryption_t encryption;

  if (argc != 3)
  {
    fprintf(stderr, "usage: %s PRINTER_URI FILE.pdf\n", argv[0]);
    return 64;
  }

  if (httpSeparateURI(HTTP_URI_CODING_ALL, argv[1],
                      scheme, sizeof(scheme),
                      userpass, sizeof(userpass),
                      host, sizeof(host),
                      &port, resource, sizeof(resource)) < HTTP_URI_STATUS_OK ||
      (strcmp(scheme, "ipp") && strcmp(scheme, "ipps")))
  {
    fputs("invalid IPP printer URI\n", stderr);
    return 64;
  }

  encryption = !strcmp(scheme, "ipps") ? HTTP_ENCRYPTION_ALWAYS
                                         : HTTP_ENCRYPTION_IF_REQUESTED;
  http = httpConnect2(host, port, NULL, AF_UNSPEC, encryption, 1, 30000, NULL);
  if (!http)
  {
    fprintf(stderr, "unable to connect to %s:%d\n", host, port);
    return 1;
  }

  request = ippNewRequest(IPP_OP_PRINT_JOB);
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_CHARSET,
               "attributes-charset", NULL, "utf-8");
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_LANGUAGE,
               "attributes-natural-language", NULL, "en");
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_URI,
               "printer-uri", NULL, argv[1]);
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_NAME,
               "requesting-user-name", NULL, "foliorelay-ci");
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_NAME,
               "job-name", NULL, "FolioRelay native libcups probe");
  ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_MIMETYPE,
               "document-format", NULL, "application/pdf");

  response = cupsDoFileRequest(http, request, resource, argv[2]);
  if (!response || cupsLastError() > IPP_STATUS_OK_CONFLICT)
  {
    fprintf(stderr, "Print-Job failed: %s\n", cupsLastErrorString());
    if (response)
      ippDelete(response);
    httpClose(http);
    return 1;
  }

  job_id = ippFindAttribute(response, "job-id", IPP_TAG_INTEGER);
  printf("status=%s job-id=%d\n",
         ippErrorString(ippGetStatusCode(response)),
         job_id ? ippGetInteger(job_id, 0) : 0);

  ippDelete(response);
  httpClose(http);
  return 0;
}
