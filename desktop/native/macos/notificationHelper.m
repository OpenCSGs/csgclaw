#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

@interface CSGClawNotificationHelper : NSObject <NSApplicationDelegate, UNUserNotificationCenterDelegate>
@property(strong) NSMutableData *input;
@end

@implementation CSGClawNotificationHelper
- (void)emit:(NSDictionary *)message {
  dispatch_async(dispatch_get_main_queue(), ^{
    NSData *data = [NSJSONSerialization dataWithJSONObject:message options:0 error:nil];
    [NSFileHandle.fileHandleWithStandardOutput writeData:data];
    [NSFileHandle.fileHandleWithStandardOutput writeData:[@"\n" dataUsingEncoding:NSUTF8StringEncoding]];
  });
}
- (void)reply:(NSString *)request result:(NSDictionary *)result {
  NSMutableDictionary *message = [result mutableCopy];
  message[@"request_id"] = request;
  [self emit:message];
}
- (NSString *)permission:(UNNotificationSettings *)settings {
  switch (settings.authorizationStatus) {
    case UNAuthorizationStatusNotDetermined: return @"default";
    case UNAuthorizationStatusDenied: return @"denied";
    case UNAuthorizationStatusAuthorized:
    case UNAuthorizationStatusProvisional: return @"granted";
    default: return @"unsupported";
  }
}
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
  self.input = [NSMutableData data];
  UNUserNotificationCenter.currentNotificationCenter.delegate = self;
  [self emit:@{@"event": @"ready"}];
  NSFileHandle.fileHandleWithStandardInput.readabilityHandler = ^(NSFileHandle *file) {
    NSData *data = file.availableData;
    dispatch_async(dispatch_get_main_queue(), ^{
      if (!data.length) { [NSApp terminate:nil]; return; }
      [self.input appendData:data];
      NSData *newline = [@"\n" dataUsingEncoding:NSUTF8StringEncoding];
      while (YES) {
        NSRange delimiter = [self.input rangeOfData:newline options:0 range:NSMakeRange(0, self.input.length)];
        if (delimiter.location == NSNotFound) break;
        NSData *line = [self.input subdataWithRange:NSMakeRange(0, delimiter.location)];
        [self.input replaceBytesInRange:NSMakeRange(0, delimiter.location + 1) withBytes:NULL length:0];
        id message = [NSJSONSerialization JSONObjectWithData:line options:0 error:nil];
        if ([message isKindOfClass:NSDictionary.class]) [self handle:message];
      }
      if (self.input.length > 32768) [NSApp terminate:nil];
    });
  };
}
- (void)handle:(NSDictionary *)message {
  NSString *request = message[@"request_id"];
  NSString *operation = message[@"operation"];
  if (![request isKindOfClass:NSString.class] || ![operation isKindOfClass:NSString.class]) return;
  UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
  if ([operation isEqual:@"permission"]) {
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      [self reply:request result:@{@"ok": @YES, @"permission": [self permission:settings]}];
    }];
  } else if ([operation isEqual:@"request_permission"]) {
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound completionHandler:^(BOOL granted, NSError *error) {
      [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
        [self reply:request result:@{@"ok": @YES, @"permission": [self permission:settings]}];
      }];
    }];
  } else if ([operation isEqual:@"show"]) {
    id input = message[@"notification"];
    if (![input isKindOfClass:NSDictionary.class]) {
      [self reply:request result:@{@"ok": @NO, @"error": @"invalid_notification"}]; return;
    }
    NSString *identifier = input[@"id"];
    if (![identifier isKindOfClass:NSString.class] || ![identifier hasPrefix:@"csgclaw-"] ||
        ![input[@"title"] isKindOfClass:NSString.class] || ![input[@"body"] isKindOfClass:NSString.class] ||
        ![input[@"roomID"] isKindOfClass:NSString.class] || ![input[@"silent"] isKindOfClass:NSNumber.class]) {
      [self reply:request result:@{@"ok": @NO, @"error": @"invalid_notification"}]; return;
    }
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      if (![[self permission:settings] isEqual:@"granted"]) {
        [self reply:request result:@{@"ok": @NO, @"error": @"notification_permission_required"}]; return;
      }
      UNMutableNotificationContent *content = [UNMutableNotificationContent new];
      content.title = input[@"title"];
      content.body = input[@"body"];
      content.sound = [input[@"silent"] boolValue] ? nil : UNNotificationSound.defaultSound;
      content.userInfo = @{@"room_id": input[@"roomID"]};
      UNNotificationRequest *notification = [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:nil];
      [center addNotificationRequest:notification withCompletionHandler:^(NSError *error) {
        [self reply:request result:error ? @{@"ok": @NO, @"error": @"notification_delivery_failed"} : @{@"ok": @YES}];
      }];
    }];
  } else if ([operation isEqual:@"close"]) {
    NSString *identifier = message[@"notification_id"];
    if ([identifier isKindOfClass:NSString.class]) [center removeDeliveredNotificationsWithIdentifiers:@[identifier]];
    [self reply:request result:@{@"ok": @YES}];
  } else {
    [self reply:request result:@{@"ok": @NO, @"error": @"unknown_operation"}];
  }
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))completion {
  if ([response.actionIdentifier isEqual:UNNotificationDefaultActionIdentifier]) {
    [self emit:@{@"event": @"click", @"notification_id": response.notification.request.identifier}];
  }
  completion();
}
@end

int main(void) {
  @autoreleasepool {
    NSApplication *app = NSApplication.sharedApplication;
    [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
    CSGClawNotificationHelper *helper = [CSGClawNotificationHelper new];
    app.delegate = helper;
    [app run];
  }
}
