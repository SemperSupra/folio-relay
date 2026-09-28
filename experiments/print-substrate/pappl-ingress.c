// FolioRelay PAPPL ingress qualification fixture.
//
// This is deliberately a thin, non-production bridge used to prove that PAPPL
// can expose the original spooled PDF to FolioRelay without owning durable
// product state or requiring a PAPPL fork.
#include <pappl/pappl.h>
#include <errno.h>
#include <spawn.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

extern char **environ;

static const char *const override_names[] = {
  "FOLIORELAY_SUBSTRATE",
  "FOLIORELAY_SOURCE_JOB_ID",
  "FOLIORELAY_MEDIA_TYPE",
  "FOLIORELAY_COPIES"
};

static bool env_has_name(const char *entry, const char *name)
{
  size_t n = strlen(name);
  return !strncmp(entry, name, n) && entry[n] == '=';
}

static char **make_child_env(const char *job_id, const char *format, int copies)
{
  size_t inherited = 0, kept = 0, i, j;
  char **envp;
  char jobbuf[64], copiesbuf[64];

  snprintf(jobbuf, sizeof(jobbuf), "FOLIORELAY_SOURCE_JOB_ID=%s", job_id);
  snprintf(copiesbuf, sizeof(copiesbuf), "FOLIORELAY_COPIES=%d", copies > 0 ? copies : 1);

  while (environ[inherited])
    inherited++;

  envp = calloc(inherited + 5, sizeof(char *));
  if (!envp)
    return NULL;

  for (i = 0; i < inherited; i++)
  {
    bool override = false;
    for (j = 0; j < sizeof(override_names) / sizeof(override_names[0]); j++)
    {
      if (env_has_name(environ[i], override_names[j]))
      {
        override = true;
        break;
      }
    }
    if (!override)
      envp[kept++] = environ[i];
  }

  if (asprintf(&envp[kept++], "FOLIORELAY_SUBSTRATE=pappl") < 0 ||
      asprintf(&envp[kept++], "%s", jobbuf) < 0 ||
      asprintf(&envp[kept++], "FOLIORELAY_MEDIA_TYPE=%s", format ? format : "application/pdf") < 0 ||
      asprintf(&envp[kept++], "%s", copiesbuf) < 0)
  {
    while (kept > inherited)
      free(envp[--kept]);
    free(envp);
    return NULL;
  }
  envp[kept] = NULL;
  return envp;
}

static void free_child_env(char **envp)
{
  size_t i, inherited = 0;
  if (!envp)
    return;
  while (environ[inherited])
    inherited++;
  for (i = 0; envp[i]; i++)
  {
    bool inherited_pointer = false;
    size_t j;
    for (j = 0; j < inherited; j++)
    {
      if (envp[i] == environ[j])
      {
        inherited_pointer = true;
        break;
      }
    }
    if (!inherited_pointer)
      free(envp[i]);
  }
  free(envp);
}

static bool bridge_printfile(pappl_job_t *job, pappl_pr_options_t *options, pappl_device_t *device)
{
  const char *adapter = getenv("FOLIORELAY_INGEST_BIN");
  const char *filename = papplJobGetFilename(job);
  const char *format = papplJobGetFormat(job);
  char idbuf[32];
  char **envp;
  char *const argv[] = {(char *)adapter, (char *)filename, NULL};
  pid_t pid;
  int rc, status;

  (void)device;

  if (!adapter || !*adapter || !filename || !*filename)
    return false;

  snprintf(idbuf, sizeof(idbuf), "%d", papplJobGetID(job));
  envp = make_child_env(idbuf, format, options ? options->copies : 1);
  if (!envp)
    return false;

  rc = posix_spawn(&pid, adapter, NULL, NULL, argv, envp);
  free_child_env(envp);
  if (rc)
    return false;

  do
  {
    rc = waitpid(pid, &status, 0);
  }
  while (rc < 0 && errno == EINTR);

  return rc == pid && WIFEXITED(status) && WEXITSTATUS(status) == 0;
}

static bool reject_raster_job(pappl_job_t *job, pappl_pr_options_t *options, pappl_device_t *device)
{
  (void)job; (void)options; (void)device;
  return false;
}
static bool reject_raster_page(pappl_job_t *job, pappl_pr_options_t *options, pappl_device_t *device, unsigned page)
{
  (void)job; (void)options; (void)device; (void)page;
  return false;
}
static bool reject_raster_line(pappl_job_t *job, pappl_pr_options_t *options, pappl_device_t *device, unsigned y, const unsigned char *line)
{
  (void)job; (void)options; (void)device; (void)y; (void)line;
  return false;
}

static bool driver_cb(
    pappl_system_t *system,
    const char *driver_name,
    const char *device_uri,
    const char *device_id,
    pappl_pr_driver_data_t *data,
    ipp_t **driver_attrs,
    void *cbdata)
{
  (void)system; (void)driver_name; (void)device_uri; (void)device_id;
  (void)driver_attrs; (void)cbdata;

  data->printfile_cb = bridge_printfile;
  data->rendjob_cb = reject_raster_job;
  data->rendpage_cb = reject_raster_page;
  data->rstartjob_cb = reject_raster_job;
  data->rstartpage_cb = reject_raster_page;
  data->rwriteline_cb = reject_raster_line;

  data->format = "application/pdf";
  snprintf(data->make_and_model, sizeof(data->make_and_model), "FolioRelay PDF Ingress");
  data->kind = PAPPL_KIND_DOCUMENT;
  data->ppm = 100;
  data->ppm_color = 100;
  data->orient_default = IPP_ORIENT_NONE;
  data->quality_default = IPP_QUALITY_NORMAL;
  data->color_supported = PAPPL_COLOR_MODE_AUTO | PAPPL_COLOR_MODE_COLOR | PAPPL_COLOR_MODE_MONOCHROME;
  data->color_default = PAPPL_COLOR_MODE_AUTO;
  data->sides_supported = PAPPL_SIDES_ONE_SIDED;
  data->sides_default = PAPPL_SIDES_ONE_SIDED;

  data->num_resolution = 1;
  data->x_resolution[0] = data->y_resolution[0] = 300;
  data->x_default = data->y_default = 300;
  data->raster_types = PAPPL_PWG_RASTER_TYPE_SRGB_8;

  data->num_media = 2;
  data->media[0] = "iso_a4_210x297mm";
  data->media[1] = "na_letter_8.5x11in";
  data->num_source = 1;
  data->source[0] = "main";
  data->num_type = 1;
  data->type[0] = "stationery";
  data->media_ready[0].size_width = 21000;
  data->media_ready[0].size_length = 29700;
  snprintf(data->media_ready[0].size_name, sizeof(data->media_ready[0].size_name), "iso_a4_210x297mm");
  snprintf(data->media_ready[0].source, sizeof(data->media_ready[0].source), "main");
  snprintf(data->media_ready[0].type, sizeof(data->media_ready[0].type), "stationery");
  data->media_default = data->media_ready[0];

  return true;
}

int main(void)
{
  static pappl_pr_driver_t drivers[] = {
    {"foliorelay-pdf", "FolioRelay PDF Ingress", "MFG:FolioRelay;MDL:PDF Ingress;CMD:PDF;", NULL}
  };
  const char *spool = getenv("FOLIORELAY_PAPPL_SPOOL");
  const char *portenv = getenv("FOLIORELAY_PAPPL_PORT");
  const char *tlsenv = getenv("FOLIORELAY_PAPPL_TLS_ONLY");
  const char *hostname = getenv("FOLIORELAY_PAPPL_HOSTNAME");
  int port = portenv ? atoi(portenv) : 8633;
  bool tls_only = tlsenv && (!strcmp(tlsenv, "1") || !strcmp(tlsenv, "true") || !strcmp(tlsenv, "yes"));
  pappl_system_t *system;
  pappl_printer_t *printer;

  if (!spool || !*spool || port < 1 || port > 65535)
  {
    fputs("invalid PAPPL fixture configuration\n", stderr);
    return 64;
  }

  system = papplSystemCreate(
      PAPPL_SOPTIONS_NONE,
      "FolioRelay PAPPL",
      port,
      NULL,
      spool,
      "-",
      PAPPL_LOGLEVEL_WARN,
      NULL,
      tls_only);
  if (!system)
    return 1;

  if (hostname && *hostname)
    papplSystemSetHostName(system, hostname);

  papplSystemSetPrinterDrivers(system, 1, drivers, NULL, NULL, driver_cb, NULL);
  papplSystemAddListeners(system, NULL);

  printer = papplPrinterCreate(
      system,
      1,
      "FolioRelay",
      "foliorelay-pdf",
      "MFG:FolioRelay;MDL:PDF Ingress;CMD:PDF;",
      "file:/dev/null");
  if (!printer)
    return 1;

  // PAPPL queue/history is transient ingress state. FolioRelay owns durable state.
  papplPrinterSetMaxActiveJobs(printer, 1000);
  papplPrinterSetMaxPreservedJobs(printer, 0);
  papplPrinterSetMaxCompletedJobs(printer, 100);

  papplSystemRun(system);
  return 0;
}
