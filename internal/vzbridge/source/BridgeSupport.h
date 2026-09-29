#pragma once
#include <dispatch/dispatch.h>
#include <stddef.h>
void *VZBridgeQueuePointer(dispatch_queue_t queue);
#include <stdbool.h>
void VZBridgeStartWindow(void *machine, void *queue, double width, double height, const char *title, bool enableController);
