#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <linux/fs.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/ioctl.h>
#include <sys/stat.h>
#include <sys/sysmacros.h>
#include <unistd.h>

/* Only an explicitly selected 4KiB block of the verified root partition. */
int main(int argc, char **argv) {
    char *end;
    if (argc != 2) return 2;
    errno = 0;
    unsigned long long block = strtoull(argv[1], &end, 10);
    if (errno || !*argv[1] || *end || block >= 15505494ULL) return 2;
    if (block != 15505492ULL && block != 15505493ULL && block != 15503361ULL &&
        block != 15503362ULL && block != 15503363ULL &&
        block != 15503874ULL && block != 15503875ULL) return 2;
    int fd = open("/dev/sda2", O_RDONLY | O_CLOEXEC);
    struct stat st;
    unsigned long long bytes = 0;
    if (fd < 0 || fstat(fd, &st) || !S_ISBLK(st.st_mode) ||
        major(st.st_rdev) != 8 || minor(st.st_rdev) != 2 ||
        ioctl(fd, BLKGETSIZE64, &bytes) || bytes != 63510503424ULL) {
        fprintf(stderr, "REFUSE: verified device geometry unavailable\n");
        if (fd >= 0) close(fd);
        return 2;
    }
    unsigned char buffer[4096];
    ssize_t result = pread(fd, buffer, sizeof(buffer), (off_t)(block * 4096ULL));
    close(fd);
    if (result != (ssize_t)sizeof(buffer)) {
        fprintf(stderr, "REFUSE: incomplete metadata block read\n");
        return 1;
    }
    return fwrite(buffer, 1, sizeof(buffer), stdout) == sizeof(buffer) &&
           fflush(stdout) == 0 ? 0 : 1;
}
