INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/Assist .
	cp anvillm-notify $INSTALL_PATH/anvillm-notify
	chmod +x $INSTALL_PATH/anvillm-notify

install:V: build

clean:V:
	rm -f $INSTALL_PATH/Assist
