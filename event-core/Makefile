CC ?= cc
CFLAGS ?= -O2 -pipe
CFLAGS += -std=c11 -Wall -Wextra -Wpedantic -Werror
CPPFLAGS += -D_POSIX_C_SOURCE=200809L -Iinclude
LDFLAGS ?=

TARGET = event-core
TEST_TARGET = protocol-test
SOURCES = src/main.c
TEST_SOURCES = tests/protocol_test.c

SOAK_DURATION ?= 300
SOAK_SUBSCRIBERS ?= 16
SOAK_RATE ?= 10

.PHONY: all test soak clean

all: $(TARGET)

$(TARGET): $(SOURCES) include/event_protocol.h
	$(CC) $(CPPFLAGS) $(CFLAGS) $(SOURCES) -o $(TARGET) $(LDFLAGS)

$(TEST_TARGET): $(TEST_SOURCES) $(SOURCES) include/event_protocol.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -DPP_IPC_TESTING $(TEST_SOURCES) -o $(TEST_TARGET) $(LDFLAGS)

test: $(TARGET) $(TEST_TARGET)
	./$(TEST_TARGET)
	python3 tests/integration_test.py ./$(TARGET)

soak: $(TARGET)
	python3 tests/soak_test.py ./$(TARGET) --duration $(SOAK_DURATION) --subscribers $(SOAK_SUBSCRIBERS) --rate $(SOAK_RATE)

clean:
	rm -f $(TARGET) $(TEST_TARGET)
