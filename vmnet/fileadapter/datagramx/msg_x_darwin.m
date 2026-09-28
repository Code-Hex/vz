#import "msg_x_darwin.h"

// MARK: - helpers

struct msghdr_x *allocateMsgHdrXArray(int count)
{
    size_t totalSize = sizeof(struct msghdr_x) * count;
    struct msghdr_x *msgHdrs = (struct msghdr_x *)malloc(totalSize);
    memset(msgHdrs, 0, totalSize);
    return msgHdrs;
}

void deallocateMsgHdrXArray(struct msghdr_x *msgHdrs)
{
    free(msgHdrs);
}
