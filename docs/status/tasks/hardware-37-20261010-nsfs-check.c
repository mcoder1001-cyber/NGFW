#define _GNU_SOURCE
/* Read-only namespace inspection. setns affects only a short-lived child. */
#include <errno.h>
#include <fcntl.h>
#include <linux/nsfs.h>
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/stat.h>
#include <sys/statfs.h>
#include <sys/wait.h>
#include <unistd.h>

int main(int argc, char **argv) {
    unsigned major, minor;
    if (argc != 4 || sscanf(argv[2], "%u", &major) != 1 ||
        sscanf(argv[3], "%u", &minor) != 1) return 2;
    int fd = open(argv[1], O_RDONLY | O_CLOEXEC);
    struct statfs fs;
    struct stat st;
    if (fd < 0 || fstatfs(fd, &fs) || fstat(fd, &st) ||
        (unsigned long)fs.f_type != 0x6e736673UL) {
        fprintf(stderr, "REFUSE: namespace path unavailable or not nsfs\n");
        if (fd >= 0) close(fd);
        return 2;
    }
    int type = ioctl(fd, NS_GET_NSTYPE);
    if (type < 0) { perror("NS_GET_NSTYPE"); close(fd); return 2; }
    printf("namespace inode=%lu type=0x%x\n", (unsigned long)st.st_ino, type);
    fflush(stdout);
    if (type != CLONE_NEWNS) { close(fd); return 0; }
    pid_t child = fork();
    if (child < 0) { perror("fork"); close(fd); return 2; }
    if (child == 0) {
        if (setns(fd, CLONE_NEWNS)) { perror("setns"); _exit(2); }
        close(fd);
        FILE *fp = fopen("/proc/self/mountinfo", "re");
        if (!fp) { perror("mountinfo"); _exit(2); }
        char *line = NULL;
        size_t capacity = 0;
        unsigned entries = 0, oldroot = 0, nested = 0;
        int malformed = 0;
        while (getline(&line, &capacity, fp) >= 0) {
            unsigned a, b;
            char *separator = strstr(line, " - ");
            if (!separator || sscanf(line, "%*u %*u %u:%u", &a, &b) != 2) {
                malformed = 1; continue;
            }
            entries++;
            if (a == major && b == minor) oldroot++;
            /* No unexamined namespace-file mount can be silently accepted. */
            if (!strncmp(separator + 3, "nsfs ", 5)) nested++;
        }
        if (ferror(fp) || !entries) malformed = 1;
        free(line);
        fclose(fp);
        printf("mount_entries=%u oldroot_mounts=%u unresolved_nsfs_mounts=%u malformed=%d\n",
               entries, oldroot, nested, malformed);
        fflush(stdout);
        _exit(malformed ? 2 : oldroot ? 3 : nested ? 4 : 0);
    }
    close(fd);
    int status;
    if (waitpid(child, &status, 0) != child || !WIFEXITED(status)) return 2;
    return WEXITSTATUS(status);
}
