// EventKit bridge: reads/writes the calendars macOS already syncs
// (Google, iCloud, Exchange… added in System Settings → Internet Accounts).
// Every function returns a malloc'd UTF-8 string the Go side frees.

#import <EventKit/EventKit.h>
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>

extern void goStoreChanged(void);

static EKEventStore *store;

static char *cstr(NSString *s) { return strdup(s ? s.UTF8String : ""); }

static NSString *toJSON(id obj) {
    NSData *d = [NSJSONSerialization dataWithJSONObject:obj options:0 error:nil];
    return d ? [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] : @"[]";
}

static NSString *str(const char *c) { return c ? [NSString stringWithUTF8String:c] : @""; }

static NSString *hexColor(CGColorRef c) {
    if (!c) return @"#4169E1";
    NSColor *col = [[NSColor colorWithCGColor:c] colorUsingColorSpace:[NSColorSpace sRGBColorSpace]];
    if (!col) return @"#4169E1";
    return [NSString stringWithFormat:@"#%02X%02X%02X",
            (int)lround(col.redComponent * 255), (int)lround(col.greenComponent * 255),
            (int)lround(col.blueComponent * 255)];
}

static BOOL looksGoogle(NSString *s) {
    s = s.lowercaseString ?: @"";
    return [s containsString:@"google"] || [s containsString:@"gmail"] || [s containsString:@"googlemail"];
}

// Google accounts added in Internet Accounts show up as CalDAV sources titled with the
// account email (or "Google"); the primary calendar is usually named after the email too.
static BOOL isGoogle(EKCalendar *c) {
    EKSource *src = c.source;
    if (!src) return NO;
    if (looksGoogle(src.title) || looksGoogle(c.title)) return YES;
    return NO;
}

static NSString *sourceTypeName(EKSourceType t) {
    switch (t) {
        case EKSourceTypeLocal: return @"local";
        case EKSourceTypeExchange: return @"exchange";
        case EKSourceTypeCalDAV: return @"caldav";
        case EKSourceTypeMobileMe: return @"icloud";
        case EKSourceTypeSubscribed: return @"subscribed";
        case EKSourceTypeBirthdays: return @"birthdays";
        default: return @"other";
    }
}

void ek_init(void) {
    if (store) return;
    store = [[EKEventStore alloc] init];
    [[NSNotificationCenter defaultCenter] addObserverForName:EKEventStoreChangedNotification
                                                      object:store
                                                       queue:nil
                                                  usingBlock:^(NSNotification *n) { goStoreChanged(); }];
}

// 0 notDetermined, 1 restricted, 2 denied, 3 full access, 4 write-only
int ek_status(void) {
    return (int)[EKEventStore authorizationStatusForEntityType:EKEntityTypeEvent];
}

int ek_request(void) {
    ek_init();
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block BOOL ok = NO;
    if (@available(macOS 14.0, *)) {
        [store requestFullAccessToEventsWithCompletion:^(BOOL granted, NSError *e) {
            ok = granted;
            dispatch_semaphore_signal(sem);
        }];
    } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [store requestAccessToEntityType:EKEntityTypeEvent completion:^(BOOL granted, NSError *e) {
            ok = granted;
            dispatch_semaphore_signal(sem);
        }];
#pragma clang diagnostic pop
    }
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(180 * NSEC_PER_SEC)));
    if (ok) {
        [store reset];
        [store refreshSourcesIfNecessary];
    }
    return ok ? 1 : 0;
}

void ek_refresh(void) {
    ek_init();
    [store refreshSourcesIfNecessary];
}

char *ek_calendars(void) {
    ek_init();
    EKCalendar *def = [store defaultCalendarForNewEvents];
    NSMutableArray *out = [NSMutableArray array];
    for (EKCalendar *c in [store calendarsForEntityType:EKEntityTypeEvent]) {
        [out addObject:@{
            @"id": c.calendarIdentifier ?: @"",
            @"name": c.title ?: @"",
            @"color": hexColor(c.CGColor),
            @"writable": c.allowsContentModifications ? @YES : @NO,
            // `&&` yields an int, which would serialise as 1/0 instead of true/false
            @"primary": (def && [c.calendarIdentifier isEqualToString:def.calendarIdentifier]) ? @YES : @NO,
            @"account": c.source.title ?: @"",
            @"google": isGoogle(c) ? @YES : @NO,
            @"sourceType": sourceTypeName(c.source.sourceType),
        }];
    }
    return cstr(toJSON(out));
}

// hiddenJSON: JSON array of calendar IDs to skip.
char *ek_events(double from, double to, const char *hiddenJSON) {
    ek_init();
    NSArray *hidden = @[];
    NSData *hd = [str(hiddenJSON) dataUsingEncoding:NSUTF8StringEncoding];
    if (hd.length) {
        id h = [NSJSONSerialization JSONObjectWithData:hd options:0 error:nil];
        if ([h isKindOfClass:[NSArray class]]) hidden = h;
    }
    NSMutableArray *cals = [NSMutableArray array];
    for (EKCalendar *c in [store calendarsForEntityType:EKEntityTypeEvent]) {
        if (![hidden containsObject:c.calendarIdentifier]) [cals addObject:c];
    }
    if (cals.count == 0) return cstr(@"[]");

    NSPredicate *p = [store predicateForEventsWithStartDate:[NSDate dateWithTimeIntervalSince1970:from]
                                                    endDate:[NSDate dateWithTimeIntervalSince1970:to]
                                                  calendars:cals];
    NSArray *evs = [[store eventsMatchingPredicate:p] sortedArrayUsingSelector:@selector(compareStartDateWithEvent:)];
    NSCalendar *greg = [NSCalendar currentCalendar];
    NSMutableArray *out = [NSMutableArray array];
    for (EKEvent *e in evs) {
        if (e.status == EKEventStatusCanceled) continue;
        BOOL declined = NO;
        for (EKParticipant *pp in e.attendees) {
            if (pp.isCurrentUser && pp.participantStatus == EKParticipantStatusDeclined) declined = YES;
        }
        if (declined) continue;

        NSDate *s = e.startDate, *en = e.endDate;
        if (e.allDay) { // normalise to [midnight, next midnight)
            s = [greg startOfDayForDate:e.startDate];
            NSDate *lastDay = [greg startOfDayForDate:[e.endDate dateByAddingTimeInterval:-1]];
            if ([lastDay compare:s] == NSOrderedAscending) lastDay = s;
            en = [greg dateByAddingUnit:NSCalendarUnitDay value:1 toDate:lastDay options:0];
        }
        NSString *notes = e.notes ?: @"";
        if (notes.length > 4000) notes = [notes substringToIndex:4000];
        [out addObject:@{
            @"id": e.eventIdentifier ?: @"",
            @"calendarId": e.calendar.calendarIdentifier ?: @"",
            @"calendar": e.calendar.title ?: @"",
            @"color": hexColor(e.calendar.CGColor),
            @"title": e.title ?: @"",
            @"location": e.location ?: @"",
            @"start": @(s.timeIntervalSince1970),
            @"end": @(en.timeIntervalSince1970),
            @"allDay": e.allDay ? @YES : @NO,
            @"url": e.URL.absoluteString ?: @"",
            @"notes": notes,
        }];
    }
    return cstr(toJSON(out));
}

// Returns "" on success, otherwise an error message. end is exclusive for all-day events.
char *ek_add(const char *title, const char *location, const char *notes, const char *calendarId,
             double start, double end, int allDay, int alarmMinutes) {
    ek_init();
    EKEvent *e = [EKEvent eventWithEventStore:store];
    e.title = str(title);
    if (location && location[0]) e.location = str(location);
    if (notes && notes[0]) e.notes = str(notes);
    EKCalendar *cal = nil;
    if (calendarId && calendarId[0]) cal = [store calendarWithIdentifier:str(calendarId)];
    if (!cal || !cal.allowsContentModifications) cal = [store defaultCalendarForNewEvents];
    if (!cal) return cstr(@"No calendar to add events to — add an account in System Settings → Internet Accounts");
    e.calendar = cal;
    e.allDay = allDay ? YES : NO;
    e.startDate = [NSDate dateWithTimeIntervalSince1970:start];
    double endT = allDay ? fmax(start, end - 1) : end;
    e.endDate = [NSDate dateWithTimeIntervalSince1970:endT];
    if (alarmMinutes > 0) [e addAlarm:[EKAlarm alarmWithRelativeOffset:-60.0 * alarmMinutes]];
    NSError *err = nil;
    if (![store saveEvent:e span:EKSpanThisEvent commit:YES error:&err]) {
        return cstr(err.localizedDescription ?: @"Couldn't save the event");
    }
    return cstr(@"");
}

// Finds the occurrence of a (possibly recurring) event that starts nearest `start`.
static EKEvent *findOccurrence(NSString *ident, double start) {
    NSDate *s = [NSDate dateWithTimeIntervalSince1970:start];
    NSPredicate *p = [store predicateForEventsWithStartDate:[s dateByAddingTimeInterval:-2 * 86400]
                                                    endDate:[s dateByAddingTimeInterval:2 * 86400]
                                                  calendars:nil];
    EKEvent *target = nil;
    double best = 1e18;
    for (EKEvent *e in [store eventsMatchingPredicate:p]) {
        if (![e.eventIdentifier isEqualToString:ident]) continue;
        double d = fabs(e.startDate.timeIntervalSince1970 - start);
        if (d < best) { best = d; target = e; }
    }
    if (!target) target = [store eventWithIdentifier:ident];
    return target;
}

static NSString *statusName(EKParticipantStatus st) {
    switch (st) {
        case EKParticipantStatusAccepted: return @"accepted";
        case EKParticipantStatusDeclined: return @"declined";
        case EKParticipantStatusTentative: return @"maybe";
        case EKParticipantStatusPending: return @"pending";
        default: return @"unknown";
    }
}

// Full details for the event card: notes, attendees, organizer, recurrence.
char *ek_detail(const char *eventId, double start) {
    ek_init();
    EKEvent *e = findOccurrence(str(eventId), start);
    if (!e) return cstr(@"{\"error\":\"Event not found\"}");
    NSMutableArray *people = [NSMutableArray array];
    for (EKParticipant *pp in e.attendees) {
        NSString *email = @"";
        NSString *u = pp.URL.absoluteString ?: @"";
        if ([u hasPrefix:@"mailto:"]) email = [u substringFromIndex:7];
        [people addObject:@{
            @"name": pp.name ?: @"",
            @"email": email,
            @"status": statusName(pp.participantStatus),
            @"me": pp.isCurrentUser ? @YES : @NO,
        }];
    }
    NSString *org = @"";
    if (e.organizer) org = e.organizer.name ?: @"";
    NSCalendar *greg = [NSCalendar currentCalendar];
    NSDate *s = e.startDate, *en = e.endDate;
    if (e.allDay) {
        s = [greg startOfDayForDate:e.startDate];
        NSDate *lastDay = [greg startOfDayForDate:[e.endDate dateByAddingTimeInterval:-1]];
        if ([lastDay compare:s] == NSOrderedAscending) lastDay = s;
        en = [greg dateByAddingUnit:NSCalendarUnitDay value:1 toDate:lastDay options:0];
    }
    NSDictionary *d = @{
        @"id": e.eventIdentifier ?: @"",
        @"calendarId": e.calendar.calendarIdentifier ?: @"",
        @"calendar": e.calendar.title ?: @"",
        @"color": hexColor(e.calendar.CGColor),
        @"title": e.title ?: @"",
        @"location": e.location ?: @"",
        @"notes": e.notes ?: @"",
        @"url": e.URL.absoluteString ?: @"",
        @"start": @(s.timeIntervalSince1970),
        @"end": @(en.timeIntervalSince1970),
        @"allDay": e.allDay ? @YES : @NO,
        @"recurring": e.hasRecurrenceRules ? @YES : @NO,
        @"writable": e.calendar.allowsContentModifications ? @YES : @NO,
        @"organizer": org,
        @"attendees": people,
    };
    return cstr(toJSON(d));
}

// Edits one occurrence. end is exclusive for all-day events. Returns "" on success.
char *ek_update(const char *eventId, double origStart, const char *title, const char *location,
                const char *notes, const char *calendarId, double start, double end, int allDay) {
    ek_init();
    EKEvent *e = findOccurrence(str(eventId), origStart);
    if (!e) return cstr(@"Event not found — it may already be gone");
    if (!e.calendar.allowsContentModifications) return cstr(@"That calendar is read-only");
    e.title = str(title);
    e.location = str(location);
    e.notes = str(notes);
    if (calendarId && calendarId[0] && ![str(calendarId) isEqualToString:e.calendar.calendarIdentifier]) {
        EKCalendar *cal = [store calendarWithIdentifier:str(calendarId)];
        if (cal && cal.allowsContentModifications) e.calendar = cal;
    }
    e.allDay = allDay ? YES : NO;
    e.startDate = [NSDate dateWithTimeIntervalSince1970:start];
    e.endDate = [NSDate dateWithTimeIntervalSince1970:(allDay ? fmax(start, end - 1) : end)];
    NSError *err = nil;
    if (![store saveEvent:e span:EKSpanThisEvent commit:YES error:&err]) {
        return cstr(err.localizedDescription ?: @"Couldn't save the change");
    }
    return cstr(@"");
}

// Deletes one occurrence (the one starting nearest `start`). Returns "" on success.
char *ek_delete(const char *eventId, double start) {
    ek_init();
    EKEvent *target = findOccurrence(str(eventId), start);
    if (!target) return cstr(@"Event not found — it may already be gone");
    if (!target.calendar.allowsContentModifications) return cstr(@"That calendar is read-only");
    NSError *err = nil;
    if (![store removeEvent:target span:EKSpanThisEvent commit:YES error:&err]) {
        return cstr(err.localizedDescription ?: @"Couldn't delete the event");
    }
    return cstr(@"");
}
