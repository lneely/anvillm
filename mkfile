INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/Assist .

install:V: build

clean:V:
	rm -f $INSTALL_PATH/Assist
