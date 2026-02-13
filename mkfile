INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/Assist .
	cp anvillm-notify $INSTALL_PATH/anvillm-notify
	chmod +x $INSTALL_PATH/anvillm-notify
	mkdir -p $INSTALL_PATH/Workflows
	cp scripts/DevReview $INSTALL_PATH/Workflows/DevReview
	chmod +x $INSTALL_PATH/Workflows/DevReview
	cp scripts/Planning $INSTALL_PATH/Workflows/Planning
	chmod +x $INSTALL_PATH/Workflows/Planning

install:V: build

clean:V:
	rm -f $INSTALL_PATH/Assist
