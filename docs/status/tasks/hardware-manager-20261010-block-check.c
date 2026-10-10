/* Task-only Linux offline block-device guard. Never writes to the device. */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/stat.h>
#include <sys/sysmacros.h>
#include <unistd.h>

static int parse_device_number(const char *s, unsigned int *result) {
    char *end;
    errno = 0;
    unsigned long n = strtoul(s, &end, 10);
    if (errno || !*s || *end || n > UINT_MAX) return -1;
    *result = (unsigned int)n;
    return 0;
}

int main(int argc, char **argv) {
    struct stat st;
    unsigned int expected_major, expected_minor;
    if (argc != 4 || parse_device_number(argv[2], &expected_major) ||
        parse_device_number(argv[3], &expected_minor)) {
        fputs("usage: block-check DEVICE EXPECTED_MAJOR EXPECTED_MINOR\n", stderr);
        return 2;
    }
    if (lstat(argv[1], &st) || !S_ISBLK(st.st_mode) ||
        major(st.st_rdev) != expected_major || minor(st.st_rdev) != expected_minor) {
        fputs("refused: target is not the expected direct block device\n", stderr);
        return 2;
    }
    int fd = open(argv[1], O_RDONLY | O_EXCL | O_CLOEXEC | O_NOFOLLOW);
    if (fd < 0) {
        if (errno == EBUSY) {
            puts("BUSY: block device still has an exclusive holder; no repair permitted");
            return 3;
        }
        perror("exclusive read-only open");
        return 2;
    }
    if (fstat(fd, &st) || !S_ISBLK(st.st_mode) ||
        major(st.st_rdev) != expected_major || minor(st.st_rdev) != expected_minor) {
        fputs("refused: opened device identity changed\n", stderr);
        close(fd);
        return 2;
    }
    if (close(fd)) {
        perror("close exclusive device probe");
        return 2;
    }
    puts("EXCLUSIVE_OPEN_OK: no filesystem/block holder; also require namespace/reference audit");
    return 0;
}
