#import <UIKit/UIKit.h>

@interface AppDelegate : UIResponder <UIApplicationDelegate>
@end

static NSURL *ResultURL(void)
{
  NSURL *docs = [[[NSFileManager defaultManager]
      URLsForDirectory:NSDocumentDirectory
             inDomains:NSUserDomainMask] firstObject];
  return [docs URLByAppendingPathComponent:@"result.json"];
}

static void WriteResult(NSDictionary *result)
{
  NSError *error = nil;
  NSData *json = [NSJSONSerialization dataWithJSONObject:result
                                                  options:NSJSONWritingPrettyPrinted
                                                    error:&error];
  if (json && !error)
    [json writeToURL:ResultURL() atomically:YES];
}

@implementation AppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions
{
  NSURL *pdfURL = [[NSBundle mainBundle] URLForResource:@"probe" withExtension:@"pdf"];
  NSData *pdf = pdfURL ? [NSData dataWithContentsOfURL:pdfURL] : nil;

  BOOL printingAvailable = [UIPrintInteractionController isPrintingAvailable];
  BOOL canPrintPDF = pdf ? [UIPrintInteractionController canPrintData:pdf] : NO;

  UIPrintInteractionController *controller =
      [UIPrintInteractionController sharedPrintController];

  NSMutableDictionary *result = [@{
    @"printing_available": @(printingAvailable),
    @"can_print_pdf": @(canPrintPDF),
    @"controller_constructed": @(controller != nil),
    @"simulator_model": [[UIDevice currentDevice] model] ?: @"",
    @"system_name": [[UIDevice currentDevice] systemName] ?: @"",
    @"system_version": [[UIDevice currentDevice] systemVersion] ?: @"",
    @"probe_pdf_present": @(pdf != nil)
  } mutableCopy];

  NSString *printerURI =
      [[[NSProcessInfo processInfo] environment] objectForKey:@"FOLIORELAY_PRINTER_URI"];

  if (printerURI.length == 0) {
    UIPrinter *explicitPrinter =
        [UIPrinter printerWithURL:[NSURL URLWithString:@"ipp://127.0.0.1:8633/ipp/print"]];
    result[@"explicit_ui_printer_constructed"] = @(explicitPrinter != nil);
    WriteResult(result);
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)),
                   dispatch_get_main_queue(), ^{
      exit(0);
    });
    return YES;
  }

  UIPrinter *printer = [UIPrinter printerWithURL:[NSURL URLWithString:printerURI]];
  result[@"printer_uri"] = printerURI;
  result[@"explicit_ui_printer_constructed"] = @(printer != nil);

  if (!printer || !pdf || !controller) {
    result[@"printer_contact_available"] = @NO;
    result[@"print_completed"] = @NO;
    result[@"error"] = @"missing printer, PDF, or print controller";
    WriteResult(result);
    exit(2);
  }

  [printer contactPrinter:^(BOOL available) {
    dispatch_async(dispatch_get_main_queue(), ^{
      result[@"printer_contact_available"] = @(available);
      if (!available) {
        result[@"print_completed"] = @NO;
        result[@"error"] = @"UIPrinter contactPrinter returned unavailable";
        WriteResult(result);
        exit(3);
      }

      UIPrintInfo *info = [UIPrintInfo printInfo];
      info.jobName = @"FolioRelay iOS Simulator native probe";
      info.outputType = UIPrintInfoOutputGeneral;
      controller.printInfo = info;
      controller.printingItem = pdf;

      [controller printToPrinter:printer
              completionHandler:^(UIPrintInteractionController *printController,
                                  BOOL completed,
                                  NSError *error) {
        result[@"print_completed"] = @(completed);
        result[@"error"] = error ? [error description] : @"";
        WriteResult(result);
        exit(completed && !error ? 0 : 4);
      }];
    });
  }];

  return YES;
}

@end

int main(int argc, char *argv[])
{
  @autoreleasepool {
    return UIApplicationMain(argc, argv, nil, NSStringFromClass([AppDelegate class]));
  }
}
