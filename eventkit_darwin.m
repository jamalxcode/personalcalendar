// EventKit bridge: reads/writes the calendars macOS already syncs
// (Google, iCloud, Exchange… added in System Settings → Internet Accounts).
// Every function returns a malloc'd UTF-8 string the Go side frees.

#import <EventKit/EventKit.h>
#import <AppKit/AppKit.h>
#import <MapKit/MapKit.h>
#import <CoreLocation/CoreLocation.h>
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
    // bring us forward so macOS shows its permission prompt on top
    dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
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
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(60 * NSEC_PER_SEC)));
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
            @"timeZone": e.timeZone.name ?: @"",
            @"recurring": e.hasRecurrenceRules ? @YES : @NO,
        }];
    }
    return cstr(toJSON(out));
}

static NSDictionary *parseJSON(const char *c) {
    NSData *d = [str(c) dataUsingEncoding:NSUTF8StringEncoding];
    id o = d.length ? [NSJSONSerialization JSONObjectWithData:d options:0 error:nil] : nil;
    return [o isKindOfClass:[NSDictionary class]] ? o : @{};
}
static NSString *sval(NSDictionary *d, NSString *k) { id v = d[k]; return [v isKindOfClass:[NSString class]] ? v : @""; }
static double dval(NSDictionary *d, NSString *k) { id v = d[k]; return [v respondsToSelector:@selector(doubleValue)] ? [v doubleValue] : 0; }
static BOOL bval(NSDictionary *d, NSString *k) { id v = d[k]; return [v respondsToSelector:@selector(boolValue)] ? [v boolValue] : NO; }

// repeat: {freq: daily|weekly|monthly|yearly, interval, weekdays:[0..6 Sun..Sat], until: unix (0 = none), count}
static EKRecurrenceRule *ruleFrom(NSDictionary *r) {
    NSString *f = sval(r, @"freq");
    EKRecurrenceFrequency freq;
    if ([f isEqualToString:@"daily"]) freq = EKRecurrenceFrequencyDaily;
    else if ([f isEqualToString:@"weekly"]) freq = EKRecurrenceFrequencyWeekly;
    else if ([f isEqualToString:@"monthly"]) freq = EKRecurrenceFrequencyMonthly;
    else if ([f isEqualToString:@"yearly"]) freq = EKRecurrenceFrequencyYearly;
    else return nil;
    NSInteger interval = MAX(1, (NSInteger)dval(r, @"interval"));
    NSMutableArray *days = nil;
    id wd = r[@"weekdays"];
    if (freq == EKRecurrenceFrequencyWeekly && [wd isKindOfClass:[NSArray class]] && [wd count]) {
        days = [NSMutableArray array];
        for (id n in wd) {
            NSInteger k = [n integerValue];
            if (k >= 0 && k <= 6) [days addObject:[EKRecurrenceDayOfWeek dayOfWeek:(EKWeekday)(k + 1)]];
        }
    }
    EKRecurrenceEnd *end = nil;
    double until = dval(r, @"until");
    NSInteger count = (NSInteger)dval(r, @"count");
    if (until > 0) end = [EKRecurrenceEnd recurrenceEndWithEndDate:[NSDate dateWithTimeIntervalSince1970:until]];
    else if (count > 0) end = [EKRecurrenceEnd recurrenceEndWithOccurrenceCount:count];
    return [[EKRecurrenceRule alloc] initRecurrenceWithFrequency:freq interval:interval daysOfTheWeek:days
                                                   daysOfTheMonth:nil monthsOfTheYear:nil weeksOfTheYear:nil
                                                    daysOfTheYear:nil setPositions:nil end:end];
}

static NSDictionary *ruleTo(EKEvent *e) {
    EKRecurrenceRule *r = e.recurrenceRules.firstObject;
    if (!r) return @{@"freq": @"none", @"interval": @1, @"weekdays": @[], @"until": @0, @"count": @0};
    NSString *f = @[@"daily", @"weekly", @"monthly", @"yearly"][MIN((NSInteger)r.frequency, 3)];
    NSMutableArray *days = [NSMutableArray array];
    for (EKRecurrenceDayOfWeek *d in r.daysOfTheWeek) [days addObject:@((NSInteger)d.dayOfTheWeek - 1)];
    return @{@"freq": f, @"interval": @(r.interval), @"weekdays": days,
             @"until": @(r.recurrenceEnd.endDate ? r.recurrenceEnd.endDate.timeIntervalSince1970 : 0),
             @"count": @(r.recurrenceEnd.occurrenceCount)};
}

// Applies the editable fields from JSON onto an event.
static void applyFields(EKEvent *e, NSDictionary *d, BOOL isNew) {
    e.title = sval(d, @"title");
    e.location = sval(d, @"location");
    e.notes = sval(d, @"notes");
    NSString *cid = sval(d, @"calendarId");
    if (cid.length && ![cid isEqualToString:e.calendar.calendarIdentifier]) {
        EKCalendar *cal = [store calendarWithIdentifier:cid];
        if (cal && cal.allowsContentModifications) e.calendar = cal;
    }
    BOOL allDay = bval(d, @"allDay");
    double start = dval(d, @"start"), end = dval(d, @"end");
    e.allDay = allDay;
    NSString *tz = sval(d, @"timeZone");
    e.timeZone = (!allDay && tz.length && [NSTimeZone timeZoneWithName:tz]) ? [NSTimeZone timeZoneWithName:tz]
                                                                            : (allDay ? nil : [NSTimeZone localTimeZone]);
    e.startDate = [NSDate dateWithTimeIntervalSince1970:start];
    e.endDate = [NSDate dateWithTimeIntervalSince1970:(allDay ? fmax(start, end - 1) : end)];
    if (bval(d, @"setAlarm")) {
        for (EKAlarm *a in [e.alarms copy]) [e removeAlarm:a];
        if (bval(d, @"hasAlarm")) [e addAlarm:[EKAlarm alarmWithRelativeOffset:dval(d, @"alarmSecs")]];
    }
    if (isNew || bval(d, @"setRepeat")) {
        id r = d[@"repeat"];
        EKRecurrenceRule *rule = [r isKindOfClass:[NSDictionary class]] ? ruleFrom(r) : nil;
        e.recurrenceRules = rule ? @[rule] : nil;
    }
}

// Adds an event from JSON. Returns {"id": "...", "start": unix} or {"error": "..."}.
char *ek_add_json(const char *json) {
    ek_init();
    NSDictionary *d = parseJSON(json);
    EKEvent *e = [EKEvent eventWithEventStore:store];
    EKCalendar *cal = nil;
    NSString *cid = sval(d, @"calendarId");
    if (cid.length) cal = [store calendarWithIdentifier:cid];
    if (!cal || !cal.allowsContentModifications) cal = [store defaultCalendarForNewEvents];
    if (!cal) return cstr(@"{\"error\":\"No calendar to add events to — add an account in System Settings → Internet Accounts\"}");
    e.calendar = cal;
    applyFields(e, d, YES);
    NSError *err = nil;
    if (![store saveEvent:e span:EKSpanThisEvent commit:YES error:&err]) {
        return cstr(toJSON(@{@"error": err.localizedDescription ?: @"Couldn't save the event"}));
    }
    return cstr(toJSON(@{@"id": e.eventIdentifier ?: @"", @"start": @(e.startDate.timeIntervalSince1970)}));
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
        @"repeat": ruleTo(e),
        @"timeZone": e.timeZone.name ?: @"",
        @"alarmSecs": e.alarms.count ? @(e.alarms.firstObject.relativeOffset) : @0,
        @"hasAlarm": e.alarms.count ? @YES : @NO,
        @"structured": e.structuredLocation.geoLocation ? @YES : @NO,
    };
    return cstr(toJSON(d));
}

// Edits an event. span 0 = this occurrence, 1 = this and all future ones. Returns "" or an error.
char *ek_update_json(const char *eventId, double origStart, const char *json, int span) {
    ek_init();
    EKEvent *e = findOccurrence(str(eventId), origStart);
    if (!e) return cstr(@"Event not found — it may already be gone");
    if (!e.calendar.allowsContentModifications) return cstr(@"That calendar is read-only");
    applyFields(e, parseJSON(json), NO);
    NSError *err = nil;
    if (![store saveEvent:e span:(span ? EKSpanFutureEvents : EKSpanThisEvent) commit:YES error:&err]) {
        return cstr(err.localizedDescription ?: @"Couldn't save the change");
    }
    return cstr(@"");
}

// Deletes one occurrence (the one starting nearest `start`). Returns "" on success.
char *ek_delete(const char *eventId, double start, int span) {
    ek_init();
    EKEvent *target = findOccurrence(str(eventId), start);
    if (!target) return cstr(@"Event not found — it may already be gone");
    if (!target.calendar.allowsContentModifications) return cstr(@"That calendar is read-only");
    NSError *err = nil;
    if (![store removeEvent:target span:(span ? EKSpanFutureEvents : EKSpanThisEvent) commit:YES error:&err]) {
        return cstr(err.localizedDescription ?: @"Couldn't delete the event");
    }
    return cstr(@"");
}


// ======================= Reminders (to-dos) =======================

int ek_rem_status(void) {
    return (int)[EKEventStore authorizationStatusForEntityType:EKEntityTypeReminder];
}

int ek_rem_request(void) {
    ek_init();
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block BOOL ok = NO;
    if (@available(macOS 14.0, *)) {
        [store requestFullAccessToRemindersWithCompletion:^(BOOL granted, NSError *e) { ok = granted; dispatch_semaphore_signal(sem); }];
    } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [store requestAccessToEntityType:EKEntityTypeReminder completion:^(BOOL granted, NSError *e) { ok = granted; dispatch_semaphore_signal(sem); }];
#pragma clang diagnostic pop
    }
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(180 * NSEC_PER_SEC)));
    if (ok) [store reset];
    return ok ? 1 : 0;
}

char *ek_rem_lists(void) {
    ek_init();
    EKCalendar *def = [store defaultCalendarForNewReminders];
    NSMutableArray *out = [NSMutableArray array];
    for (EKCalendar *c in [store calendarsForEntityType:EKEntityTypeReminder]) {
        [out addObject:@{@"id": c.calendarIdentifier ?: @"", @"name": c.title ?: @"", @"color": hexColor(c.CGColor),
                         @"primary": (def && [def.calendarIdentifier isEqualToString:c.calendarIdentifier]) ? @YES : @NO}];
    }
    return cstr(toJSON(out));
}

static NSDictionary *remDict(EKReminder *r) {
    NSDate *due = nil;
    BOOL hasTime = NO;
    if (r.dueDateComponents) {
        NSDateComponents *dc = r.dueDateComponents;
        hasTime = dc.hour != NSDateComponentUndefined;
        NSCalendar *cal = [NSCalendar currentCalendar];
        due = [cal dateFromComponents:dc];
    }
    return @{@"id": r.calendarItemIdentifier ?: @"", @"title": r.title ?: @"", @"notes": r.notes ?: @"",
             @"list": r.calendar.title ?: @"", @"listId": r.calendar.calendarIdentifier ?: @"",
             @"color": hexColor(r.calendar.CGColor), @"done": r.completed ? @YES : @NO,
             @"due": @(due ? due.timeIntervalSince1970 : 0), @"hasTime": hasTime ? @YES : @NO,
             @"priority": @(r.priority)};
}

// Incomplete reminders (all lists, max 200).
char *ek_reminders(void) {
    ek_init();
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block NSArray *items = @[];
    NSPredicate *p = [store predicateForIncompleteRemindersWithDueDateStarting:nil ending:nil calendars:nil];
    [store fetchRemindersMatchingPredicate:p completion:^(NSArray<EKReminder *> *rs) {
        items = rs ?: @[];
        dispatch_semaphore_signal(sem);
    }];
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(20 * NSEC_PER_SEC)));
    NSMutableArray *out = [NSMutableArray array];
    for (EKReminder *r in items) {
        if (out.count >= 200) break;
        [out addObject:remDict(r)];
    }
    return cstr(toJSON(out));
}

// Adds (no id) or updates a reminder: {id, title, notes, listId, due (unix, 0 = none), hasTime, done}.
char *ek_rem_save(const char *json) {
    ek_init();
    NSDictionary *d = parseJSON(json);
    NSString *rid = sval(d, @"id");
    EKReminder *r = nil;
    if (rid.length) {
        r = (EKReminder *)[store calendarItemWithIdentifier:rid];
        if (![r isKindOfClass:[EKReminder class]]) return cstr(@"{\"error\":\"To-do not found\"}");
    } else {
        r = [EKReminder reminderWithEventStore:store];
        EKCalendar *cal = nil;
        NSString *lid = sval(d, @"listId");
        if (lid.length) cal = [store calendarWithIdentifier:lid];
        r.calendar = cal ?: [store defaultCalendarForNewReminders];
        if (!r.calendar) return cstr(@"{\"error\":\"No Reminders list available\"}");
    }
    if (d[@"title"]) r.title = sval(d, @"title");
    if (d[@"notes"]) r.notes = sval(d, @"notes");
    if (d[@"due"]) {
        double due = dval(d, @"due");
        for (EKAlarm *a in [r.alarms copy]) [r removeAlarm:a];
        if (due > 0) {
            NSCalendarUnit u = NSCalendarUnitYear | NSCalendarUnitMonth | NSCalendarUnitDay;
            if (bval(d, @"hasTime")) u |= NSCalendarUnitHour | NSCalendarUnitMinute;
            NSDate *dd = [NSDate dateWithTimeIntervalSince1970:due];
            r.dueDateComponents = [[NSCalendar currentCalendar] components:u fromDate:dd];
            if (bval(d, @"hasTime")) [r addAlarm:[EKAlarm alarmWithAbsoluteDate:dd]];
        } else {
            r.dueDateComponents = nil;
        }
    }
    if (d[@"done"]) r.completed = bval(d, @"done");
    NSError *err = nil;
    if (![store saveReminder:r commit:YES error:&err]) {
        return cstr(toJSON(@{@"error": err.localizedDescription ?: @"Couldn't save the to-do"}));
    }
    return cstr(toJSON(remDict(r)));
}

char *ek_rem_delete(const char *rid) {
    ek_init();
    EKReminder *r = (EKReminder *)[store calendarItemWithIdentifier:str(rid)];
    if (![r isKindOfClass:[EKReminder class]]) return cstr(@"To-do not found");
    NSError *err = nil;
    if (![store removeReminder:r commit:YES error:&err]) return cstr(err.localizedDescription ?: @"Couldn't delete the to-do");
    return cstr(@"");
}

// ======================= Clipboard (text or image) =======================

// Returns {"text": "...", "image": "<base64 jpeg>"} — images are scaled to max 1600px.
char *clip_read(void) {
    __block NSString *text = @"", *img = @"";
    void (^work)(void) = ^{
        NSPasteboard *pb = [NSPasteboard generalPasteboard];
        NSImage *image = [[NSImage alloc] initWithPasteboard:pb];
        if (image && image.representations.count) {
            NSSize sz = image.size;
            CGFloat scale = MIN(1.0, 1600.0 / MAX(sz.width, sz.height));
            NSInteger w = MAX(1, (NSInteger)(sz.width * scale)), h = MAX(1, (NSInteger)(sz.height * scale));
            NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:w pixelsHigh:h
                bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSCalibratedRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
            [NSGraphicsContext saveGraphicsState];
            [NSGraphicsContext setCurrentContext:[NSGraphicsContext graphicsContextWithBitmapImageRep:rep]];
            [[NSColor whiteColor] setFill];
            NSRectFill(NSMakeRect(0, 0, w, h));
            [image drawInRect:NSMakeRect(0, 0, w, h) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1.0];
            [NSGraphicsContext restoreGraphicsState];
            NSData *jpg = [rep representationUsingType:NSBitmapImageFileTypeJPEG properties:@{NSImageCompressionFactor: @0.82}];
            if (jpg) img = [jpg base64EncodedStringWithOptions:0];
        }
        NSString *t = [pb stringForType:NSPasteboardTypeString];
        if (t) text = t;
    };
    if ([NSThread isMainThread]) work(); else dispatch_sync(dispatch_get_main_queue(), work);
    return cstr(toJSON(@{@"text": text, @"image": img}));
}

// ======================= Travel time (Apple Maps) =======================

static CLLocation *originLoc = nil;
static NSString *originAddr = nil;

static CLLocation *geocodeSync(NSString *addr, CLLocation *near) {
    if (!addr.length) return nil;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block CLLocation *loc = nil;
    CLGeocoder *g = [[CLGeocoder alloc] init];
    void (^done)(NSArray<CLPlacemark *> *, NSError *) = ^(NSArray<CLPlacemark *> *pl, NSError *e) {
        loc = pl.firstObject.location;
        dispatch_semaphore_signal(sem);
    };
    if (near) {
        CLCircularRegion *reg = [[CLCircularRegion alloc] initWithCenter:near.coordinate radius:100000 identifier:@"near"];
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [g geocodeAddressString:addr inRegion:reg completionHandler:done];
#pragma clang diagnostic pop
    } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [g geocodeAddressString:addr completionHandler:done];
#pragma clang diagnostic pop
    }
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(15 * NSEC_PER_SEC)));
    return loc;
}

// The event's place as {"lat","lon","loc"} — a quick store lookup (called under the calendar lock).
char *ek_event_place(const char *eventId, double start) {
    ek_init();
    EKEvent *e = findOccurrence(str(eventId), start);
    if (!e) return strdup("{}");
    CLLocation *g = e.structuredLocation.geoLocation;
    NSDictionary *d = @{@"lat": @(g ? g.coordinate.latitude : 0), @"lon": @(g ? g.coordinate.longitude : 0), @"loc": e.location ?: @""};
    NSData *j = [NSJSONSerialization dataWithJSONObject:d options:0 error:nil];
    return strdup([[[NSString alloc] initWithData:j encoding:NSUTF8StringEncoding] UTF8String]);
}

static NSMutableDictionary<NSString *, CLLocation *> *destCache = nil;

// Driving time in seconds from `from` (an address) to a place, or -1 if unknown. Slow (network):
// never called under the calendar lock. Only the travel loop calls it, so the caches need no lock.
double ek_eta(const char *from, double lat, double lon, const char *place) {
    NSString *f = str(from), *p = str(place);
    if (!f.length) return -1;
    if (!originLoc || ![originAddr isEqualToString:f]) {
        originLoc = geocodeSync(f, nil);
        originAddr = f;
    }
    if (!originLoc) return -1;
    CLLocation *dest = nil;
    if (lat != 0 || lon != 0) dest = [[CLLocation alloc] initWithLatitude:lat longitude:lon];
    if (!dest && p.length) {
        if (!destCache) destCache = [NSMutableDictionary dictionary];
        dest = destCache[p];
        if (!dest) { dest = geocodeSync(p, originLoc); if (dest) destCache[p] = dest; } // Apple rate-limits geocoding: cache places
    }
    if (!dest) return -1;
    if ([dest distanceFromLocation:originLoc] < 300) return 0;
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block double secs = -1;
    dispatch_async(dispatch_get_main_queue(), ^{
        MKDirectionsRequest *req = [[MKDirectionsRequest alloc] init];
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        req.source = [[MKMapItem alloc] initWithPlacemark:[[MKPlacemark alloc] initWithCoordinate:originLoc.coordinate]];
        req.destination = [[MKMapItem alloc] initWithPlacemark:[[MKPlacemark alloc] initWithCoordinate:dest.coordinate]];
#pragma clang diagnostic pop
        req.transportType = MKDirectionsTransportTypeAutomobile;
        req.departureDate = [NSDate date];
        MKDirections *dir = [[MKDirections alloc] initWithRequest:req];
        [dir calculateETAWithCompletionHandler:^(MKETAResponse *resp, NSError *err) {
            if (resp) secs = resp.expectedTravelTime;
            dispatch_semaphore_signal(sem);
        }];
    });
    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(20 * NSEC_PER_SEC)));
    return secs;
}

// ---- menu bar item: hide the icon while there's text ----
static NSButton *findStatusButton(NSView *v) {
    if ([NSStringFromClass([v class]) containsString:@"StatusBarButton"]) return (NSButton *)v;
    for (NSView *s in v.subviews) { NSButton *b = findStatusButton(s); if (b) return b; }
    return nil;
}

void tray_set_image(const void *data, int len, int tmpl) {
    NSData *d = [NSData dataWithBytes:data length:len];
    dispatch_async(dispatch_get_main_queue(), ^{ dispatch_async(dispatch_get_main_queue(), ^{ dispatch_async(dispatch_get_main_queue(), ^{
        NSImage *img = [[NSImage alloc] initWithData:d];
        CGFloat t = [[NSStatusBar systemStatusBar] thickness];
        [img setSize:NSMakeSize(t, t)];
        [img setTemplate:tmpl ? YES : NO];
        for (NSWindow *w in [NSApp windows]) {
            if (![NSStringFromClass([w class]) containsString:@"StatusBar"]) continue;
            NSButton *b = findStatusButton(w.contentView);
            if (!b && [w.contentView isKindOfClass:[NSButton class]]) b = (NSButton *)w.contentView;
            if (b) b.image = img;
        }
    }); }); });
}

void tray_show_image(int show) {
    // queued a few hops deep so it lands after any icon change Wails has queued
    dispatch_async(dispatch_get_main_queue(), ^{ dispatch_async(dispatch_get_main_queue(), ^{ dispatch_async(dispatch_get_main_queue(), ^{
        for (NSWindow *w in [NSApp windows]) {
            if (![NSStringFromClass([w class]) containsString:@"StatusBar"]) continue;
            NSButton *b = findStatusButton(w.contentView);
            if (!b && [w.contentView isKindOfClass:[NSButton class]]) b = (NSButton *)w.contentView;
            if (b) b.imagePosition = show ? NSImageLeft : NSNoImage;
        }
    }); }); });
}

// ---- menu bar item: remembered position, and whether macOS is actually showing it ----
// The status bar window is NSStatusBarWindow; its statusItem isn't public API, so read it carefully.
static NSStatusItem *ourStatusItem(void) {
    for (NSWindow *w in [NSApp windows]) {
        if (![NSStringFromClass([w class]) containsString:@"StatusBar"]) continue;
        @try {
            id it = [w valueForKey:@"statusItem"];
            if ([it isKindOfClass:[NSStatusItem class]]) return (NSStatusItem *)it;
        } @catch (NSException *e) {}
    }
    return nil;
}

static void onMain(dispatch_block_t b) {
    if ([NSThread isMainThread]) b(); else dispatch_sync(dispatch_get_main_queue(), b);
}

// Give the item a saved name so macOS remembers where it sits (after you ⌘-drag it) across
// launches and updates — without one, every launch puts it at the far left, the first spot
// macOS hides behind the notch when the menu bar is full. Also make sure it isn't switched off.
void tray_autosave(void) {
    onMain(^{
        NSStatusItem *it = ourStatusItem();
        if (!it) return;
        if (![it.autosaveName isEqualToString:@"SmartCalStatusItem"]) it.autosaveName = @"SmartCalStatusItem";
        if (!it.visible) it.visible = YES;
    });
}

// 1 when the item is really on screen; 0 when macOS hides it (full menu bar / behind the notch); -1 unknown.
int tray_visible(void) {
    __block int r = -1;
    onMain(^{
        for (NSWindow *w in [NSApp windows]) {
            if (![NSStringFromClass([w class]) containsString:@"StatusBar"]) continue;
            if (!findStatusButton(w.contentView) && ![w.contentView isKindOfClass:[NSButton class]]) continue;
            if (!w.isVisible || !(w.occlusionState & NSWindowOcclusionStateVisible)) { r = 0; return; }
            NSRect f = w.frame;
            NSScreen *s = w.screen;
            if (!s || f.size.width < 1 || !NSContainsRect(s.frame, f)) { r = 0; return; }
            if (@available(macOS 12.0, *)) {
                NSRect right = s.auxiliaryTopRightArea, left = s.auxiliaryTopLeftArea;
                if (!NSIsEmptyRect(right) || !NSIsEmptyRect(left)) { // a notch: only the sides of it are visible
                    BOOL ok = (!NSIsEmptyRect(right) && NSMinX(f) >= NSMinX(right) - 1) ||
                              (!NSIsEmptyRect(left) && NSMaxX(f) <= NSMaxX(left) + 1);
                    r = ok ? 1 : 0; return;
                }
            }
            r = 1; return;
        }
    });
    return r;
}

// ---- Mac conventions ----
void show_about(const char *version, const char *credits) {
    NSString *v = [NSString stringWithUTF8String:version];
    NSString *c = [NSString stringWithUTF8String:credits];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSMutableParagraphStyle *ps = [[NSMutableParagraphStyle alloc] init];
        ps.alignment = NSTextAlignmentCenter;
        NSAttributedString *cr = [[NSAttributedString alloc] initWithString:c attributes:@{
            NSFontAttributeName: [NSFont systemFontOfSize:11], NSParagraphStyleAttributeName: ps,
            NSForegroundColorAttributeName: [NSColor secondaryLabelColor]}];
        [NSApp activateIgnoringOtherApps:YES];
        [NSApp orderFrontStandardAboutPanelWithOptions:@{
            @"ApplicationName": @"Smart Cal", @"ApplicationVersion": v, @"Version": @"",
            @"Credits": cr, @"Copyright": @"© 2026 go.sala.company"}];
        for (NSWindow *w in [NSApp windows]) {
            if ([NSStringFromClass([w class]) containsString:@"About"] || [w.title containsString:@"About"]) {
                [w setLevel:NSFloatingWindowLevel]; [w makeKeyAndOrderFront:nil];
            }
        }
    });
}

void set_dock_visible(int show) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:show ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory];
        if (show) [NSApp activateIgnoringOtherApps:YES];
    });
}

void app_activate(void) {
    dispatch_async(dispatch_get_main_queue(), ^{ [NSApp activateIgnoringOtherApps:YES]; });
}

// system look & locale: {"accent":"#RRGGBB","h24":true,"firstWeekday":0..6,"reduceMotion":bool,"contrast":bool,"lang":"en"}
char *sys_prefs(void) {
    __block NSString *out = @"{}";
    void (^work)(void) = ^{
        NSColor *c = [[NSColor controlAccentColor] colorUsingColorSpace:[NSColorSpace sRGBColorSpace]];
        NSString *hex = [NSString stringWithFormat:@"#%02X%02X%02X", (int)lround(c.redComponent * 255), (int)lround(c.greenComponent * 255), (int)lround(c.blueComponent * 255)];
        NSString *fmt = [NSDateFormatter dateFormatFromTemplate:@"j" options:0 locale:[NSLocale currentLocale]];
        BOOL h24 = [fmt rangeOfString:@"a"].location == NSNotFound;
        NSInteger fw = [[NSCalendar currentCalendar] firstWeekday] - 1; // 1=Sun → 0
        NSWorkspace *ws = [NSWorkspace sharedWorkspace];
        NSString *lang = [[NSLocale preferredLanguages] firstObject] ?: @"en";
        NSDictionary *d = @{@"accent": hex, @"h24": h24 ? @YES : @NO, @"firstWeekday": @(fw),
            @"reduceMotion": ws.accessibilityDisplayShouldReduceMotion ? @YES : @NO,
            @"contrast": ws.accessibilityDisplayShouldIncreaseContrast ? @YES : @NO, @"lang": lang};
        NSData *j = [NSJSONSerialization dataWithJSONObject:d options:0 error:nil];
        out = [[NSString alloc] initWithData:j encoding:NSUTF8StringEncoding];
    };
    if ([NSThread isMainThread]) work(); else dispatch_sync(dispatch_get_main_queue(), work);
    return strdup([out UTF8String]);
}
