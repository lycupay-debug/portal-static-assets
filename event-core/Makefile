CC ?= cc
CFLAGS ?= -O2 -pipe
CFLAGS += -std=c11 -Wall -Wextra -Wpedantic -Werror
CPPFLAGS += -D_POSIX_C_SOURCE=200809L -Iinclude
LDFLAGS ?=

TARGET = event-core
SOURCES = src/main.c

.PHONY: all clean test

all: $(TARGET)

$(TARGET): $(SOURCES) include/event_protocol.h
	$(CC) $(CPPFLAGS) $(CFLAGS) $(SOURCES) -o $(TARGET) $(LDFLAGS)

test:
	@tmp=$$(mktemp /var/tmp/personal-portal-event-core-test-bin.XXXXXX); $(CC) $(CPPFLAGS) $(CFLAGS) $(SOURCES) -o "$$tmp" $(LDFLAGS); status=0; EVENT_CORE_BINARY="$$tmp" sh tests/test_event_core.sh || status=$$?; /bin/rm -f "$$tmp"; exit $$status

clean:
	rm -f $(TARGET)
