CC ?= cc
CFLAGS ?= -O2 -pipe
CFLAGS += -std=c11 -Wall -Wextra -Wpedantic -Werror
CPPFLAGS += -D_POSIX_C_SOURCE=200809L
LDFLAGS ?=

TARGET = event-core
SOURCES = src/main.c

.PHONY: all clean test

all: $(TARGET)

$(TARGET): $(SOURCES)
	$(CC) $(CPPFLAGS) $(CFLAGS) $(SOURCES) -o $(TARGET) $(LDFLAGS)

test: $(TARGET)
	sh tests/test_event_core.sh

clean:
	rm -f $(TARGET)
