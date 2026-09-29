# FolioRelay

FolioRelay is a self-hosted document routing system that routes documents
between printers, files, format transformations, and delivery channels.

The project is currently being extracted from the Document Gateway candidate in
`SemperSupra/truenas-app-foundry`.  TrueNAS remains the reference deployment
and publication target, while FolioRelay owns the runtime, API/ABI contracts,
portable deployment models, and product-level qualification.

## Runtime principles

- runtime instances are stateless and disposable;
- persistent configuration and user data live outside runtime containers;
- driverless IPP is preferred over legacy printer drivers;
- renderer and sender plugins are isolated behind versioned ABIs;
- every network/privilege/package dependency must earn its keep;
- humans, automation, and agents operate on the same underlying state through
  audience-appropriate surfaces.

See:

- [Runtime image and hardening policy](docs/RUNTIME_IMAGES.md)
- [Go core ADR](docs/ADR-GO-CORE.md)
- [Hostile-document threat model](docs/THREAT_MODEL.md)
- [Security inspector ABI](docs/INSPECTOR_ABI.md)
- [Email artifact sender](docs/EMAIL_ARTIFACT_SENDER.md)
- [Red/blue team and global concept sweep](docs/RED_BLUE_GLOBAL_SWEEP.md)
