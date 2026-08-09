#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>
#import <math.h>

@interface NTermTrafficLightAligner : NSObject {
    NSWindow *_window;
    CGFloat _centerFromTop;
}
- (instancetype)initWithWindow:(NSWindow *)window centerFromTop:(CGFloat)centerFromTop;
- (void)align;
- (void)windowGeometryChanged:(NSNotification *)notification;
@end

@implementation NTermTrafficLightAligner

- (instancetype)initWithWindow:(NSWindow *)window centerFromTop:(CGFloat)centerFromTop {
    self = [super init];
    if (self) {
        _window = window;
        _centerFromTop = centerFromTop;

        NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
        [center addObserver:self selector:@selector(windowGeometryChanged:) name:NSWindowDidResizeNotification object:window];
        [center addObserver:self selector:@selector(windowGeometryChanged:) name:NSWindowDidBecomeKeyNotification object:window];
        [center addObserver:self selector:@selector(windowGeometryChanged:) name:NSWindowDidExitFullScreenNotification object:window];
        [center addObserver:self selector:@selector(windowGeometryChanged:) name:NSWindowDidChangeBackingPropertiesNotification object:window];
    }
    return self;
}

- (void)align {
    if (_window == nil || ([_window styleMask] & NSWindowStyleMaskFullScreen) != 0) {
        return;
    }

    const NSWindowButton kinds[] = {
        NSWindowCloseButton,
        NSWindowMiniaturizeButton,
        NSWindowZoomButton,
    };
    CGFloat scale = [_window backingScaleFactor];
    if (scale <= 0) {
        scale = 1;
    }

    for (NSUInteger index = 0; index < sizeof(kinds) / sizeof(kinds[0]); index++) {
        NSButton *button = [_window standardWindowButton:kinds[index]];
        NSView *container = [button superview];
        if (button == nil || container == nil) {
            continue;
        }

        NSRect frame = [button frame];
        CGFloat y;
        if ([container isFlipped]) {
            y = _centerFromTop - NSHeight(frame) / 2.0;
        } else {
            y = NSHeight([container bounds]) - _centerFromTop - NSHeight(frame) / 2.0;
        }
        frame.origin.y = round(y * scale) / scale;
        [button setFrame:frame];
    }
}

- (void)windowGeometryChanged:(NSNotification *)notification {
    (void)notification;
    [self align];
}

@end

static NTermTrafficLightAligner *NTermWindowAligner = nil;

void NTermInstallTrafficLightAlignment(double centerFromTop) {
    void (^install)(void) = ^{
        NSWindow *window = [NSApp mainWindow];
        if (window == nil) {
            window = [NSApp keyWindow];
        }
        if (window == nil) {
            return;
        }

        if (NTermWindowAligner != nil) {
            [NTermWindowAligner release];
        }
        NTermWindowAligner = [[NTermTrafficLightAligner alloc] initWithWindow:window
                                                               centerFromTop:(CGFloat)centerFromTop];
        [NTermWindowAligner align];
    };

    if ([NSThread isMainThread]) {
        install();
    } else {
        dispatch_async(dispatch_get_main_queue(), install);
    }
}
