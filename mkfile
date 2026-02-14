INSTALL_PATH=$HOME/bin

all:V: install

build:V:
	go build -o $INSTALL_PATH/anvilsrv ./cmd/anvilsrv
	go build -o $INSTALL_PATH/Assist ./cmd/Assist
	go build -o $INSTALL_PATH/anvillm ./cmd/anvillm
	cp anvillm-notify $INSTALL_PATH/anvillm-notify
	chmod 0755 $INSTALL_PATH/anvillm-notify
	mkdir -p $INSTALL_PATH/Workflows
	cp scripts/DevReview $INSTALL_PATH/Workflows/DevReview
	chmod 0755 $INSTALL_PATH/Workflows/DevReview
	cp scripts/DevReviewQA $INSTALL_PATH/Workflows/DevReviewQA
	chmod 0755 $INSTALL_PATH/Workflows/DevReviewQA
	cp scripts/Planning $INSTALL_PATH/Workflows/Planning
	chmod 0755 $INSTALL_PATH/Workflows/Planning
	mkdir -p $HOME/.emacs.d/lisp
	cp anvillm.el $HOME/.emacs.d/lisp/anvillm.el

install:V: build

clean:V:
	rm -f $INSTALL_PATH/anvilsrv
	rm -f $INSTALL_PATH/Assist
	rm -f $INSTALL_PATH/anvillm-notify
