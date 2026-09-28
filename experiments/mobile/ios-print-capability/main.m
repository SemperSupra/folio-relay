#import <UIKit/UIKit.h>

@interface AppDelegate : UIResponder <UIApplicationDelegate>
@end

@implementation AppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions
{
  BOOL printingAvailable = [UIPrintInteractionController isPrintingAvailable];

  const char pdfBytes[] =
      "%PDF-1.4\n"
      "1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
      "2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
      "3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n"
      "trailer<</Root 1 0 R>>\n%%EOF\n";
  NSData *pdf = [NSData dataWithBytes:pdfBytes length:sizeof(pdfBytes) - 1];
  BOOL canPrintPDF = [UIPrintInteractionController canPrintData:pdf];

  UIPrinter *explicitPrinter =
      [UIPrinter printerWithURL:[NSURL URLWithString:@"ipp://127.0.0.1:8633/ipp/print"]];

  UIPrintInteractionController *controller =
      [UIPrintInteractionController sharedPrintController];
  controller.printingItem = pdf;

  NSDictionary *result = @{
    @"printing_available": @(printingAvailable),
    @"can_print_pdf": @(canPrintPDF),
    @"explicit_ui_printer_constructed": @(explicitPrinter != nil),
    @"controller_constructed": @(controller != nil),
    @"simulator_model": [[UIDevice currentDevice] model] ?: @"",
    @"system_name": [[UIDevice currentDevice] systemName] ?: @"",
    @"system_version": [[UIDevice currentDevice] systemVersion] ?: @""
  };

  NSData *json = [NSJSONSerialization dataWithJSONObject:result
                                                  options:NSJSONWritingPrettyPrinted
                                                    error:nil];
  NSURL *docs = [[[NSFileManager defaultManager]
      URLsForDirectory:NSDocumentDirectory
             inDomains:NSUserDomainMask] firstObject];
  [json writeToURL:[docs URLByAppendingPathComponent:@"result.json"] atomically:YES];

  dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.5 * NSEC_PER_SEC)),
                 dispatch_get_main_queue(), ^{
    exit(0);
  });
  return YES;
}

@end

int main(int argc, char *argv[])
{
  @autoreleasepool {
    return UIApplicationMain(argc, argv, nil, NSStringFromClass([AppDelegate class]));
  }
}
