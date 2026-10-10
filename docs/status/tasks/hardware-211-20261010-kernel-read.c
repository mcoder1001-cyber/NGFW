/* Read-only kernel ring snapshot. No clear, consume, loglevel or write action. */
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/klog.h>

int main(int argc, char **argv) {
    (void)argv;
    if (argc != 1) return 2;
    int size = klogctl(10, NULL, 0); /* SYSLOG_ACTION_SIZE_BUFFER */
    if (size <= 0 || size > 64 * 1024 * 1024) {
        perror("kernel buffer size unavailable");
        return 1;
    }
    char *buffer = malloc((size_t)size);
    if (!buffer) { perror("malloc"); return 1; }
    int length = klogctl(3, buffer, size); /* SYSLOG_ACTION_READ_ALL */
    if (length < 0) { perror("kernel read unavailable"); free(buffer); return 1; }
    int result = fwrite(buffer, 1, (size_t)length, stdout) == (size_t)length &&
                 fflush(stdout) == 0 ? 0 : 1;
    free(buffer);
    return result;
}
