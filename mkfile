INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/Q .
	go build -o $INSTALL_PATH/Q-Login ./cmd/Q-Login

install:V: build

clean:V:
	rm -f $INSTALL_PATH/Q $INSTALL_PATH/Q-Login
