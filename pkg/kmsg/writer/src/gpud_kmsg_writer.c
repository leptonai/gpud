#include <linux/fs.h>
#include <linux/kernel.h>
#include <linux/module.h>
#include <linux/spinlock.h>
#include <linux/string.h>
#include <linux/uaccess.h>

#define DEVICE_NAME "gpud-kmsg-writer"
#define BUF_LEN 1000

static int major;
static char msg[BUF_LEN];
static DEFINE_MUTEX(msg_mutex);

// Function to parse log level from input and return the appropriate KERN_*
// level.
// Both of the following commands are valid:
// e.g.,
// sh -c "echo \"KERN_EMERG, System critical error!\" > /dev/gpud_kmsg_writer"
// sh -c "echo \"kern.emerg, System critical error!\" > /dev/gpud_kmsg_writer"
static const char *parse_log_level(const char *input, char *remaining_msg) {
  const char *comma_pos;
  char level_str[32];
  int level_len;

  // Find the first comma
  comma_pos = strchr(input, ',');
  if (!comma_pos) {
    // No comma found, use default level and copy entire message
    strncpy(remaining_msg, input, BUF_LEN - 1);
    remaining_msg[BUF_LEN - 1] = '\0';
    return KERN_INFO; // default level
  }

  // Extract the level part (before comma)
  level_len = comma_pos - input;
  if (level_len >= sizeof(level_str)) {
    level_len = sizeof(level_str) - 1;
  }
  strncpy(level_str, input, level_len);
  level_str[level_len] = '\0';

  // Copy the remaining message (after comma, skipping leading spaces)
  comma_pos++; // skip the comma
  while (*comma_pos == ' ' || *comma_pos == '\t') {
    comma_pos++; // skip leading whitespace
  }
  strncpy(remaining_msg, comma_pos, BUF_LEN - 1);
  remaining_msg[BUF_LEN - 1] = '\0';

  // Map level string to KERN_* constants
  // ref.
  // https://github.com/torvalds/linux/blob/master/tools/include/linux/kern_levels.h#L8-L15
  if (strcmp(level_str, "kern.emerg") == 0 ||
      strcmp(level_str, "KERN_EMERG") == 0) {
    return KERN_EMERG;
  } else if (strcmp(level_str, "kern.alert") == 0 ||
             strcmp(level_str, "KERN_ALERT") == 0) {
    return KERN_ALERT;
  } else if (strcmp(level_str, "kern.crit") == 0 ||
             strcmp(level_str, "KERN_CRIT") == 0) {
    return KERN_CRIT;
  } else if (strcmp(level_str, "kern.err") == 0 ||
             strcmp(level_str, "KERN_ERR") == 0) {
    return KERN_ERR;
  } else if (strcmp(level_str, "kern.warning") == 0 ||
             strcmp(level_str, "KERN_WARNING") == 0) {
    return KERN_WARNING;
  } else if (strcmp(level_str, "kern.notice") == 0 ||
             strcmp(level_str, "KERN_NOTICE") == 0) {
    return KERN_NOTICE;
  } else if (strcmp(level_str, "kern.info") == 0 ||
             strcmp(level_str, "KERN_INFO") == 0) {
    return KERN_INFO;
  } else if (strcmp(level_str, "kern.debug") == 0 ||
             strcmp(level_str, "KERN_DEBUG") == 0) {
    return KERN_DEBUG;
  } else {
    // Unknown level, use default
    return KERN_INFO;
  }
}

static ssize_t dev_write(struct file *file, const char __user *buf, size_t len,
                         loff_t *offset) {
  ssize_t ret;
  const char *log_level;
  char parsed_msg[BUF_LEN];

  if (len > BUF_LEN - 1)
    len = BUF_LEN - 1;

  if (!mutex_trylock(&msg_mutex)) {
    return -EBUSY;
  }

  if (copy_from_user(msg, buf, len)) {
    mutex_unlock(&msg_mutex);
    return -EFAULT;
  }

  msg[len] = '\0';

  // Parse the log level and extract the message
  log_level = parse_log_level(msg, parsed_msg);

  // Use the parsed log level in printk
  printk("%s%s\n", log_level, parsed_msg);

  ret = len;
  mutex_unlock(&msg_mutex);

  return ret;
}

static struct file_operations fops = {
    .owner = THIS_MODULE,
    .write = dev_write,
};

static int __init gpud_kmsg_writer_device_init(void) {
  major = register_chrdev(0, DEVICE_NAME, &fops);
  if (major < 0) {
    printk(KERN_ALERT "Registering dummy device failed with %d\n", major);
    return major;
  }
  printk(KERN_INFO "module loaded with device major number %d\n", major);
  return 0;
}

static void __exit gpud_kmsg_writer_device_exit(void) {
  unregister_chrdev(major, DEVICE_NAME);
  printk(KERN_INFO "char_device module unloaded\n");
}

module_init(gpud_kmsg_writer_device_init);
module_exit(gpud_kmsg_writer_device_exit);

MODULE_LICENSE("GPL");
MODULE_AUTHOR("Hitoshi Mitake, Gyuho Lee");
MODULE_DESCRIPTION("char device for injecting messages to kernel log");
