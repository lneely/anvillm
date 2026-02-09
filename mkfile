INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/Assist .
	cp scripts/Kiro $INSTALL_PATH/Kiro
	cp scripts/Claude $INSTALL_PATH/Claude
	chmod +x $INSTALL_PATH/Kiro $INSTALL_PATH/Claude

install:V: build

clean:V:
	rm -f $INSTALL_PATH/Assist $INSTALL_PATH/Kiro $INSTALL_PATH/Claude
