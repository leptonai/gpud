obj-m += gpud_kmsg_writer.o


build:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) modules


clean:
	make -C /lib/modules/$(shell uname -r)/build M=$(PWD) clean


install:
	sudo insmod gpud_kmsg_writer.ko


uninstall:
	sudo rmmod gpud_kmsg_writer
