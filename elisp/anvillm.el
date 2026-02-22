;;; anvillm.el --- Emacs interface for AnviLLM  -*- lexical-binding: t; -*-

;; Copyright (C) 2026

;; Author: Levi Neely
;; Keywords: tools, processes
;; Version: 1.0.0
;; Package-Requires: ((emacs "25.1"))

;;; Commentary:

;; Emacs interface for AnviLLM - Acme-native LLM chat backend manager.
;; Provides a session manager that interacts with anvilsrv via 9P filesystem.
;;
;; Usage:
;;   M-x anvillm
;;
;; Keybindings in session list:
;;   s - Start new session (select backend)
;;   SPC - Compose prompt in buffer
;;   M-SPC - Send prompt to selected session (minibuffer)
;;   p/n - Move up/down
;;   t - Stop selected session
;;   R - Restart selected session
;;   K - Kill selected session
;;   a - Attach to tmux session
;;   A - Set alias for selected session
;;   r - Refresh session list
;;   g - Refresh session list (standard Emacs binding)
;;   d - Daemon status
;;   q - Quit
;;   ? - Help

;;; Code:

(require 'tabulated-list)
(require 'json)

(defgroup anvillm nil
  "Emacs interface for AnviLLM."
  :group 'tools
  :prefix "anvillm-")

(defcustom anvillm-refresh-interval 2
  "Interval in seconds for auto-refreshing the session list."
  :type 'integer
  :group 'anvillm)

(defcustom anvillm-9p-command "9p"
  "Path to the 9p command from plan9port."
  :type 'string
  :group 'anvillm)

(defvar anvillm-refresh-timer nil
  "Timer for auto-refreshing the session list.")

(defvar anvillm-agent-path "agent"
  "9P filesystem path for AnviLLM agent.")

;;; 9P Filesystem Interface

(defun anvillm--9p-read (path)
  "Read from 9P filesystem PATH using the 9p command."
  (with-temp-buffer
    (let ((exit-code (call-process anvillm-9p-command nil t nil "read" path)))
      (if (zerop exit-code)
          (buffer-string)
        (error "Failed to read %s: %s" path (buffer-string))))))

(defun anvillm--9p-ls (path)
  "List directory at 9P filesystem PATH using the 9p command."
  (with-temp-buffer
    (let ((exit-code (call-process anvillm-9p-command nil t nil "ls" path)))
      (if (zerop exit-code)
          (buffer-string)
        (error "Failed to ls %s: %s" path (buffer-string))))))

(defun anvillm--9p-read-nonblocking (path callback)
  "Read from 9P filesystem PATH asynchronously, calling CALLBACK with data.
The process is killed after the first read to prevent blocking on streaming files."
  (let* ((buffer (generate-new-buffer " *9p-read*"))
         (proc (start-process "9p-read" buffer anvillm-9p-command "read" path)))
    (set-process-filter
     proc
     (lambda (process output)
       (when (buffer-live-p (process-buffer process))
         (with-current-buffer (process-buffer process)
           (goto-char (point-max))
           (insert output)))
       ;; Kill process after receiving data to prevent blocking
       (delete-process process)
       (when (buffer-live-p buffer)
         (with-current-buffer buffer
           (funcall callback (buffer-string)))
         (kill-buffer buffer))))
    (set-process-sentinel
     proc
     (lambda (process event)
       (when (buffer-live-p (process-buffer process))
         (let ((buffer (process-buffer process)))
           (with-current-buffer buffer
             (funcall callback (buffer-string)))
           (kill-buffer buffer)))))))

(defun anvillm--9p-write (path data)
  "Write DATA to 9P filesystem PATH using the 9p command."
  (with-temp-buffer
    (insert data)
    (let ((exit-code (call-process-region (point-min) (point-max)
                                          anvillm-9p-command nil t nil
                                          "write" path)))
      (unless (zerop exit-code)
        (error "Failed to write %s: %s" path (buffer-string))))))

;;; Session Management

(defun anvillm--parse-session-line (line)
  "Parse a session LINE from the 'list' file.
Format: id backend state alias cwd (whitespace-separated; often tabs)."
  (when (string-match
         "^\\([^[:space:]]+\\)\\s-+\\([^[:space:]]+\\)\\s-+\\([^[:space:]]+\\)\\s-+\\([^[:space:]]+\\)\\s-+\\(.+\\)$"
         line)
    (let ((id (match-string 1 line))
          (backend (match-string 2 line))
          (state (match-string 3 line))
          (alias (match-string 4 line))
          (cwd (match-string 5 line)))
      (when (string= alias "-") (setq alias ""))
      (list id (vector
                (substring id 0 (min 8 (length id)))
                alias
                backend
                (propertize state 'face (anvillm--state-face state))
                ""  ; PID no longer available in list output
                cwd)))))

(defun anvillm--state-face (state)
  "Return face for session STATE."
  (cond
   ((string= state "running") 'success)
   ((string= state "idle") 'font-lock-function-name-face)
   ((string= state "stopped") 'warning)
   ((or (string= state "error") (string= state "exited")) 'error)
   (t 'default)))

(defun anvillm--list-sessions ()
  "Get list of sessions from the 9P filesystem."
  (condition-case err
      (let ((list-data (anvillm--9p-read (concat anvillm-agent-path "/list"))))
        (delq nil
              (mapcar #'anvillm--parse-session-line
                      (split-string list-data "\n" t))))
    (error
     (message "Failed to list sessions: %s" (error-message-string err))
     nil)))

(defun anvillm--refresh-sessions ()
  "Refresh the session list in the current buffer."
  (when (eq major-mode 'anvillm-mode)
    (let ((sessions (anvillm--list-sessions)))
      (setq tabulated-list-entries
            (mapcar (lambda (session)
                      (list (car session) (cadr session)))
                    sessions))
      (tabulated-list-print t))))

;;; Interactive Commands

(defun anvillm-refresh ()
  "Refresh the session list."
  (interactive)
  (anvillm--refresh-sessions))

(defun anvillm-start-session ()
  "Start a new session after selecting backend."
  (interactive)
  (let ((backend (completing-read "Select backend: " '("claude" "kiro-cli") nil t)))
    (when backend
      (let ((directory (read-directory-name "Working directory: " default-directory)))
        (condition-case err
            (progn
              (anvillm--9p-write (concat anvillm-agent-path "/ctl")
                                 (format "new %s %s" backend directory))
              (message "Created %s session in %s" backend directory)
              (anvillm--refresh-sessions))
          (error
           (message "Failed to create session: %s" (error-message-string err))))))))

(defun anvillm--get-selected-session ()
  "Get the ID of the currently selected session."
  (when-let ((entry (tabulated-list-get-id)))
    entry))

(defun anvillm-stop-session ()
  "Stop the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (condition-case err
          (progn
            (anvillm--9p-write (concat anvillm-agent-path "/" session-id "/ctl") "stop")
            (message "Stopped session %s" (substring session-id 0 (min 8 (length session-id))))
            (anvillm--refresh-sessions))
        (error
         (message "Failed to stop session: %s" (error-message-string err))))
    (message "No session selected")))

(defun anvillm-restart-session ()
  "Restart the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (condition-case err
          (progn
            (anvillm--9p-write (concat anvillm-agent-path "/" session-id "/ctl") "restart")
            (message "Restarted session %s" (substring session-id 0 (min 8 (length session-id))))
            (anvillm--refresh-sessions))
        (error
         (message "Failed to restart session: %s" (error-message-string err))))
    (message "No session selected")))

(defun anvillm-kill-session ()
  "Kill the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (when (yes-or-no-p (format "Kill session %s? " (substring session-id 0 (min 8 (length session-id)))))
        (condition-case err
            (progn
              (anvillm--9p-write (concat anvillm-agent-path "/" session-id "/ctl") "kill")
              (message "Killed session %s" (substring session-id 0 (min 8 (length session-id))))
              (anvillm--refresh-sessions))
          (error
           (message "Failed to kill session: %s" (error-message-string err)))))
    (message "No session selected")))

(defun anvillm-set-alias ()
  "Set alias for the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let ((current-alias (condition-case nil
                               (string-trim (anvillm--9p-read (concat anvillm-agent-path "/" session-id "/alias")))
                             (error "")))
            (new-alias (read-string "Alias: ")))
        (condition-case err
            (progn
              (anvillm--9p-write (concat anvillm-agent-path "/" session-id "/alias") new-alias)
              (message "Set alias to '%s'" new-alias)
              (anvillm--refresh-sessions))
          (error
           (message "Failed to set alias: %s" (error-message-string err)))))
    (message "No session selected")))

(defun anvillm-assign-bead ()
  "Assign a bead to the selected agent to work on."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let* ((beads (condition-case nil
                        (let* ((json-object-type 'plist)
                               (json-array-type 'list)
                               (beads-data (anvillm--9p-read (concat anvillm-agent-path "/beads/list")))
                               (beads (json-read-from-string beads-data)))
                          (mapcar (lambda (b) (plist-get b :id)) beads))
                      (error nil)))
             (bead-id (completing-read "Bead ID: " beads nil nil)))
        (when (> (length bead-id) 0)
          (condition-case err
              (let ((msg (json-encode `((to . ,session-id)
                                       (type . "PROMPT_REQUEST")
                                       (subject . "User prompt")
                                       (body . ,(format "Work on bead %s" bead-id))))))
                (anvillm--9p-write (concat anvillm-agent-path "/user/mail") msg)
                (message "Assigned bead %s to %s" bead-id (substring session-id 0 (min 8 (length session-id)))))
            (error
             (message "Failed to assign bead: %s" (error-message-string err))))))
    (message "No session selected")))

(defun anvillm-daemon-status ()
  "Show daemon status."
  (interactive)
  (message "Checking daemon status...")
  (with-temp-buffer
    (let ((exit-code (call-process "anvilsrv" nil t nil "status")))
      (message "%s" (buffer-string)))))

(defun anvillm-help ()
  "Show help for AnviLLM mode."
  (interactive)
  (with-help-window "*AnviLLM Help*"
    (princ "AnviLLM - Emacs Interface

Keybindings:
s - Start new session (select backend)
SPC - Compose prompt in buffer (C-c C-c to send, C-c C-k to abort)
M-SPC - Send prompt (minibuffer)
l - View session log (press 'r' to refresh, 'q' to close)
c - Edit session context (C-c C-c to save, C-c C-k to abort)
i - Open inbox
C - Open archive (completed messages)
T - Open tasks/beads manager
t - Stop selected session
R - Restart selected session
K - Kill selected session
A - Set alias for selected session
a - Attach to tmux session in terminal
r, g - Refresh session list
d - Daemon status
q - Quit
? - This help

Navigation:
n, C-n - Next line
p, C-p - Previous line
RET - (reserved for future use)

Prompt Composition:
The prompt buffer supports multi-line input. Lines starting with ;;
are treated as comments and stripped before sending.

Bead ID Detection:
If you enter just a bead ID (e.g., bd-5xz), it will automatically
be wrapped with instructions to load the beads skill and work on it.

Context:
Session context is a text prefix prepended to all prompts sent to
that session. Use it to provide persistent instructions or information.

9P Filesystem:
All operations read/write the 9P filesystem at $NAMESPACE/agent

Backends:
- claude (Claude Code CLI)
- kiro-cli (Kiro CLI)

")))


;;; Inbox Management

(defvar-local anvillm--message-data nil
  "Message data for the current message view buffer.")

(defvar anvillm-message-view-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd "r") #'anvillm-message-reply)
    (define-key map (kbd "q") #'quit-window)
    map)
  "Keymap for AnviLLM message view mode.")

(define-derived-mode anvillm-message-view-mode special-mode "AnviLLM-Message"
  "Major mode for viewing AnviLLM messages.

\\{anvillm-message-view-mode-map}")

(defun anvillm-message-reply ()
  "Reply to the current message."
  (interactive)
  (unless anvillm--message-data
    (error "No message data available"))
  (let* ((from (plist-get anvillm--message-data :from))
         (subject (plist-get anvillm--message-data :subject))
         (reply-subject (if (string-prefix-p "Re: " subject)
                           subject
                         (concat "Re: " subject)))
         (buffer-name (format "*Reply to %s*" (substring from 0 (min 8 (length from)))))
         (buffer (get-buffer-create buffer-name)))
    (with-current-buffer buffer
      (anvillm-prompt-mode)
      (erase-buffer)
      (setq anvillm--prompt-session-id from)
      (insert (format ";; Reply to: %s\n" from))
      (insert (format ";; Subject: %s\n\n" reply-subject))
      (setq-local anvillm--reply-subject reply-subject))
    (pop-to-buffer buffer)
    (goto-char (point-max))))

(defvar anvillm-inbox-mode-map
  (let ((map (make-sparse-keymap)))
    (set-keymap-parent map tabulated-list-mode-map)
    (define-key map (kbd "RET") #'anvillm-inbox-view)
    (define-key map (kbd "v") #'anvillm-inbox-view)
    (define-key map (kbd "r") #'anvillm-inbox-reply)
    (define-key map (kbd "a") #'anvillm-inbox-approve)
    (define-key map (kbd "R") #'anvillm-inbox-reject)
    (define-key map (kbd "d") #'anvillm-inbox-archive)
    (define-key map (kbd "g") #'anvillm-inbox-refresh)
    (define-key map (kbd "q") #'quit-window)
    (define-key map (kbd "?") #'anvillm-inbox-help)
    map)
  "Keymap for AnviLLM inbox mode.")

(defun anvillm--list-inbox-messages ()
  "Get list of inbox messages from the 9P filesystem."
  (condition-case err
      (let* ((inbox-path (concat anvillm-agent-path "/user/inbox"))
             (files-str (anvillm--9p-ls inbox-path))
             (files (split-string files-str "\n" t))
             messages)
        (dolist (file files)
          (when (string-suffix-p ".json" file)
            (let* ((msg-path (concat inbox-path "/" file))
                   (json-object-type 'plist)
                   (json-array-type 'list)
                   (msg-json (anvillm--9p-read msg-path))
                   (msg (json-read-from-string msg-json))
                   (id (plist-get msg :id))
                   (from (plist-get msg :from))
                   (type (plist-get msg :type))
                   (subject (plist-get msg :subject))
                   (timestamp (plist-get msg :timestamp)))
              (push (list id (vector
                             (substring id 0 (min 8 (length id)))
                             (substring from 0 (min 8 (length from)))
                             type
                             subject
                             (format-time-string "%Y-%m-%d %H:%M" 
                                               (seconds-to-time timestamp))))
                    messages))))
        (nreverse messages))
    (error
     (message "Failed to list inbox: %s" (error-message-string err))
     nil)))

(defun anvillm--refresh-inbox ()
  "Refresh the inbox list in the current buffer."
  (when (eq major-mode 'anvillm-inbox-mode)
    (let ((messages (anvillm--list-inbox-messages)))
      (setq tabulated-list-entries messages)
      (tabulated-list-print t))))

(define-derived-mode anvillm-inbox-mode tabulated-list-mode "AnviLLM-Inbox"
  "Major mode for managing AnviLLM inbox.

\\{anvillm-inbox-mode-map}"
  (setq tabulated-list-format [("ID" 10 t)
                                ("From" 10 t)
                                ("Type" 20 t)
                                ("Subject" 30 t)
                                ("Time" 16 t)])
  (setq tabulated-list-padding 2)
  (setq tabulated-list-sort-key (cons "Time" t))
  (tabulated-list-init-header))

(defun anvillm-inbox ()
  "Open the AnviLLM inbox."
  (interactive)
  (let ((buffer (get-buffer-create "*AnviLLM Inbox*")))
    (with-current-buffer buffer
      (anvillm-inbox-mode)
      (anvillm--refresh-inbox))
    (switch-to-buffer buffer)))

(defun anvillm-inbox-refresh ()
  "Refresh the inbox list."
  (interactive)
  (anvillm--refresh-inbox))

(defun anvillm--get-selected-message ()
  "Get the ID of the currently selected message."
  (tabulated-list-get-id))

(defun anvillm-inbox-view ()
  "View the selected message."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (msg-path (concat anvillm-agent-path "/user/inbox/" msg-id ".json"))
                 (msg-json (anvillm--9p-read msg-path))
                 (msg (json-read-from-string msg-json))
                 (buffer (get-buffer-create (format "*Message: %s*" 
                                                   (substring msg-id 0 (min 8 (length msg-id)))))))
            (with-current-buffer buffer
              (let ((inhibit-read-only t))
                (erase-buffer)
                (insert (format "From: %s\n" (plist-get msg :from)))
                (insert (format "To: %s\n" (plist-get msg :to)))
                (insert (format "Type: %s\n" (plist-get msg :type)))
                (insert (format "Subject: %s\n" (plist-get msg :subject)))
                (insert (format "Time: %s\n" 
                              (format-time-string "%Y-%m-%d %H:%M:%S" 
                                                (seconds-to-time (plist-get msg :timestamp)))))
                (insert "\n")
                (insert (plist-get msg :body)))
              (anvillm-message-view-mode)
              (setq-local anvillm--message-data msg))
            (pop-to-buffer buffer))
        (error
         (message "Failed to view message: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm--infer-reply-type (msg-type)
  "Infer the appropriate reply type from MSG-TYPE.
Maps APPROVAL_REQUEST to APPROVAL_RESPONSE, REVIEW_REQUEST to
REVIEW_RESPONSE, and defaults to PROMPT_RESPONSE for all others."
  (cond
   ((string= msg-type "APPROVAL_REQUEST") "APPROVAL_RESPONSE")
   ((string= msg-type "REVIEW_REQUEST") "REVIEW_RESPONSE")
   (t "PROMPT_RESPONSE")))

(defun anvillm-inbox-reply ()
  "Reply to the selected message.
Automatically infers the correct reply type from the message type."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (msg-path (concat anvillm-agent-path "/user/inbox/" msg-id ".json"))
                 (msg-json (anvillm--9p-read msg-path))
                 (msg (json-read-from-string msg-json))
                 (from (plist-get msg :from))
                 (msg-type (plist-get msg :type))
                 (subject (plist-get msg :subject))
                 (reply-type (anvillm--infer-reply-type msg-type))
                 (reply-body (read-string "Reply: ")))
            (when (> (length reply-body) 0)
              (let ((reply-msg (json-encode `((to . ,from)
                                             (type . ,reply-type)
                                             (subject . ,(concat "Re: " subject))
                                             (body . ,reply-body)))))
                (anvillm--9p-write (concat anvillm-agent-path "/user/mail") reply-msg)
                (message "Sent reply to %s" from))))
        (error
         (message "Failed to reply: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm-inbox-approve ()
  "Approve the selected message.
Sends an APPROVAL_RESPONSE with body \"approved\" to the sender."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (msg-path (concat anvillm-agent-path "/user/inbox/" msg-id ".json"))
                 (msg-json (anvillm--9p-read msg-path))
                 (msg (json-read-from-string msg-json))
                 (from (plist-get msg :from))
                 (subject (plist-get msg :subject)))
            (let ((reply-msg (json-encode `((to . ,from)
                                           (type . "APPROVAL_RESPONSE")
                                           (subject . ,(concat "Re: " subject))
                                           (body . "approved")))))
              (anvillm--9p-write (concat anvillm-agent-path "/user/mail") reply-msg)
              (message "Approved message from %s" from)))
        (error
         (message "Failed to approve: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm-inbox-reject ()
  "Reject the selected message with a reason.
Prompts for a rejection reason, then sends an APPROVAL_RESPONSE
with body \"rejected: <reason>\" to the sender."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (msg-path (concat anvillm-agent-path "/user/inbox/" msg-id ".json"))
                 (msg-json (anvillm--9p-read msg-path))
                 (msg (json-read-from-string msg-json))
                 (from (plist-get msg :from))
                 (msg-type (plist-get msg :type))
                 (subject (plist-get msg :subject))
                 (reason (read-string "Rejection reason: ")))
            (when (> (length reason) 0)
              (let* ((reply-type (anvillm--infer-reply-type msg-type))
                     (reply-msg (json-encode `((to . ,from)
                                              (type . ,reply-type)
                                              (subject . ,(concat "Re: " subject))
                                              (body . ,(format "rejected: %s" reason))))))
                (anvillm--9p-write (concat anvillm-agent-path "/user/mail") reply-msg)
                (message "Rejected message from %s" from))))
        (error
         (message "Failed to reject: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm-inbox-archive ()
  "Archive (complete) the selected message."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (progn
            (anvillm--9p-write (concat anvillm-agent-path "/user/ctl")
                              (format "complete %s" msg-id))
            (message "Archived message %s" (substring msg-id 0 (min 8 (length msg-id))))
            (anvillm--refresh-inbox))
        (error
         (message "Failed to archive message: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm-inbox-help ()
  "Show help for AnviLLM inbox mode."
  (interactive)
  (with-help-window "*AnviLLM Inbox Help*"
    (princ "AnviLLM Inbox

Keybindings:
RET, v - View selected message
r - Reply to selected message (type inferred from message)
a - Approve selected message (sends APPROVAL_RESPONSE)
R - Reject selected message with reason (sends APPROVAL_RESPONSE)
d - Archive (complete) selected message
g - Refresh inbox
q - Quit
? - This help

Navigation:
n, C-n - Next line
p, C-p - Previous line

Reply Type Inference:
APPROVAL_REQUEST -> APPROVAL_RESPONSE
REVIEW_REQUEST   -> REVIEW_RESPONSE
(all others)     -> PROMPT_RESPONSE

")))

;;; Archive Management

(defvar anvillm-archive-mode-map
  (let ((map (make-sparse-keymap)))
    (set-keymap-parent map tabulated-list-mode-map)
    (define-key map (kbd "RET") #'anvillm-archive-view)
    (define-key map (kbd "v") #'anvillm-archive-view)
    (define-key map (kbd "g") #'anvillm-archive-refresh)
    (define-key map (kbd "q") #'quit-window)
    (define-key map (kbd "?") #'anvillm-archive-help)
    map)
  "Keymap for AnviLLM archive mode.")

(defun anvillm--list-archive-messages ()
  "Get list of archived (completed) messages from the 9P filesystem."
  (condition-case err
      (let* ((archive-path (concat anvillm-agent-path "/user/completed"))
             (files-str (anvillm--9p-ls archive-path))
             (files (split-string files-str "\n" t))
             messages)
        (dolist (file files)
          (when (string-suffix-p ".json" file)
            (let* ((msg-path (concat archive-path "/" file))
                   (json-object-type 'plist)
                   (json-array-type 'list)
                   (msg-json (anvillm--9p-read msg-path))
                   (msg (json-read-from-string msg-json))
                   (id (plist-get msg :id))
                   (from (plist-get msg :from))
                   (type (plist-get msg :type))
                   (subject (plist-get msg :subject))
                   (timestamp (plist-get msg :timestamp)))
              (push (list id (vector
                             (substring id 0 (min 8 (length id)))
                             (substring from 0 (min 8 (length from)))
                             type
                             subject
                             (format-time-string "%Y-%m-%d %H:%M" 
                                               (seconds-to-time timestamp))))
                    messages))))
        (nreverse messages))
    (error
     (message "Failed to list archive: %s" (error-message-string err))
     nil)))

(defun anvillm--refresh-archive ()
  "Refresh the archive list in the current buffer."
  (when (eq major-mode 'anvillm-archive-mode)
    (let ((messages (anvillm--list-archive-messages)))
      (setq tabulated-list-entries messages)
      (tabulated-list-print t))))

(define-derived-mode anvillm-archive-mode tabulated-list-mode "AnviLLM-Archive"
  "Major mode for viewing AnviLLM archived messages.

\\{anvillm-archive-mode-map}"
  (setq tabulated-list-format [("ID" 10 t)
                                ("From" 10 t)
                                ("Type" 20 t)
                                ("Subject" 30 t)
                                ("Time" 16 t)])
  (setq tabulated-list-padding 2)
  (setq tabulated-list-sort-key (cons "Time" t))
  (tabulated-list-init-header))

(defun anvillm-archive ()
  "Open the AnviLLM archive."
  (interactive)
  (let ((buffer (get-buffer-create "*AnviLLM Archive*")))
    (with-current-buffer buffer
      (anvillm-archive-mode)
      (anvillm--refresh-archive))
    (switch-to-buffer buffer)))

(defun anvillm-archive-refresh ()
  "Refresh the archive list."
  (interactive)
  (anvillm--refresh-archive))

(defun anvillm-archive-view ()
  "View the selected archived message."
  (interactive)
  (if-let ((msg-id (anvillm--get-selected-message)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (msg-path (concat anvillm-agent-path "/user/completed/" msg-id ".json"))
                 (msg-json (anvillm--9p-read msg-path))
                 (msg (json-read-from-string msg-json))
                 (buffer (get-buffer-create (format "*Archived Message: %s*" 
                                                   (substring msg-id 0 (min 8 (length msg-id)))))))
            (with-current-buffer buffer
              (let ((inhibit-read-only t))
                (erase-buffer)
                (insert (format "From: %s\n" (plist-get msg :from)))
                (insert (format "To: %s\n" (plist-get msg :to)))
                (insert (format "Type: %s\n" (plist-get msg :type)))
                (insert (format "Subject: %s\n" (plist-get msg :subject)))
                (insert (format "Time: %s\n" 
                              (format-time-string "%Y-%m-%d %H:%M:%S" 
                                                (seconds-to-time (plist-get msg :timestamp)))))
                (insert "\n")
                (insert (plist-get msg :body)))
              (anvillm-message-view-mode)
              (setq-local anvillm--message-data msg))
            (pop-to-buffer buffer))
        (error
         (message "Failed to view message: %s" (error-message-string err))))
    (message "No message selected")))

(defun anvillm-archive-help ()
  "Show help for AnviLLM archive mode."
  (interactive)
  (with-help-window "*AnviLLM Archive Help*"
    (princ "AnviLLM Archive

Keybindings:
RET, v - View selected message
g - Refresh archive
q - Quit
? - This help

Navigation:
n, C-n - Next line
p, C-p - Previous line

")))

;;; Context Management

(defvar-local anvillm--context-session-id nil
  "Session ID for the current context buffer.")

(defvar anvillm-context-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd "C-c C-c") #'anvillm-context-save)
    (define-key map (kbd "C-c C-k") #'anvillm-context-abort)
    map)
  "Keymap for AnviLLM context editing mode.")

(define-derived-mode anvillm-context-mode text-mode "AnviLLM-Context"
  "Major mode for editing session context.

\\{anvillm-context-mode-map}"
  (setq header-line-format
        (substitute-command-keys
         "Edit session context. Save: \\[anvillm-context-save] | Abort: \\[anvillm-context-abort]")))

(defun anvillm-edit-context ()
  "Edit context for the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (condition-case err
          (let* ((current-context (string-trim 
                                  (anvillm--9p-read 
                                   (concat anvillm-agent-path "/" session-id "/context"))))
                 (session-info (anvillm--get-session-info session-id))
                 (display-name (or (plist-get session-info :alias) 
                                  (substring session-id 0 (min 8 (length session-id)))))
                 (buffer-name (format "*AnviLLM Context: %s*" display-name))
                 (buffer (get-buffer-create buffer-name)))
            (with-current-buffer buffer
              (anvillm-context-mode)
              (erase-buffer)
              (setq anvillm--context-session-id session-id)
              (when (> (length current-context) 0)
                (insert current-context))
              (goto-char (point-min)))
            (pop-to-buffer buffer))
        (error
         (message "Failed to load context: %s" (error-message-string err))))
    (message "No session selected")))

(defun anvillm-context-save ()
  "Save the edited context to the session."
  (interactive)
  (unless anvillm--context-session-id
    (error "No session ID associated with this buffer"))
  (let ((context (buffer-string)))
    (condition-case err
        (progn
          (anvillm--9p-write 
           (concat anvillm-agent-path "/" anvillm--context-session-id "/context")
           context)
          (message "Context saved for %s" 
                  (substring anvillm--context-session-id 0 
                            (min 8 (length anvillm--context-session-id))))
          (quit-window t))
      (error
       (message "Failed to save context: %s" (error-message-string err))))))

(defun anvillm-context-abort ()
  "Abort context editing and close the buffer."
  (interactive)
  (when (yes-or-no-p "Discard context changes? ")
    (quit-window t)))

;;; Tasks/Beads Management

(defvar anvillm-tasks-mode-map
  (let ((map (make-sparse-keymap)))
    (set-keymap-parent map tabulated-list-mode-map)
    (define-key map (kbd "SPC") #'anvillm-tasks-new)
    (define-key map (kbd "s") #'anvillm-tasks-new-subtask)
    (define-key map (kbd "w") #'anvillm-tasks-assign-to-agent)
    (define-key map (kbd "n") #'next-line)
    (define-key map (kbd "p") #'previous-line)
    (define-key map (kbd "c") #'anvillm-tasks-claim)
    (define-key map (kbd "C") #'anvillm-tasks-complete)
    (define-key map (kbd "f") #'anvillm-tasks-fail)
    (define-key map (kbd "RET") #'anvillm-tasks-view)
    (define-key map (kbd "v") #'anvillm-tasks-view)
    (define-key map (kbd "d") #'anvillm-tasks-add-dependency)
    (define-key map (kbd "D") #'anvillm-tasks-remove-dependency)
    (define-key map (kbd "l") #'anvillm-tasks-add-label)
    (define-key map (kbd "L") #'anvillm-tasks-remove-label)
    (define-key map (kbd "m") #'anvillm-tasks-comment)
    (define-key map (kbd "r") #'anvillm-tasks-refresh)
    (define-key map (kbd "g") #'anvillm-tasks-refresh)
    (define-key map (kbd "q") #'quit-window)
    (define-key map (kbd "?") #'anvillm-tasks-help)
    map)
  "Keymap for AnviLLM tasks mode.")

(defun anvillm--parse-bead (json-str)
  "Parse a bead from JSON-STR."
  (let* ((json-object-type 'plist)
         (json-array-type 'list)
         (bead (json-read-from-string json-str)))
    bead))

(defun anvillm--list-beads ()
  "Get list of beads from the 9P filesystem."
  (condition-case err
      (let* ((json-object-type 'plist)
             (json-array-type 'list)
             (beads-data (anvillm--9p-read (concat anvillm-agent-path "/beads/ready")))
             (beads (json-read-from-string beads-data)))
        (mapcar (lambda (bead)
                  (let ((id (plist-get bead :id))
                        (title (plist-get bead :title))
                        (status (plist-get bead :status))
                        (priority (or (plist-get bead :priority) 0))
                        (assignee (or (plist-get bead :assignee) "")))
                    (list id (vector
                             id
                             (propertize status 'face (anvillm--bead-status-face status))
                             (number-to-string priority)
                             assignee
                             title))))
                beads))
    (error
     (message "Failed to list beads: %s" (error-message-string err))
     nil)))

(defun anvillm--bead-status-face (status)
  "Return face for bead STATUS."
  (cond
   ((string= status "open") 'warning)
   ((string= status "in_progress") 'font-lock-function-name-face)
   ((string= status "closed") 'success)
   (t 'default)))

(defun anvillm--refresh-tasks ()
  "Refresh the tasks list in the current buffer."
  (when (eq major-mode 'anvillm-tasks-mode)
    (let ((beads (anvillm--list-beads)))
      (setq tabulated-list-entries beads)
      (tabulated-list-print t))))

(define-derived-mode anvillm-tasks-mode tabulated-list-mode "AnviLLM-Tasks"
  "Major mode for managing AnviLLM tasks/beads.

\\{anvillm-tasks-mode-map}"
  (setq tabulated-list-format [("ID" 15 t)
                                ("Status" 12 t)
                                ("Pri" 4 t)
                                ("Assignee" 12 t)
                                ("Title" 0 t)])
  (setq tabulated-list-padding 2)
  (setq tabulated-list-sort-key (cons "ID" nil))
  (tabulated-list-init-header))

(defun anvillm-tasks ()
  "Open the AnviLLM tasks manager."
  (interactive)
  (let ((buffer (get-buffer-create "*AnviLLM Tasks*")))
    (with-current-buffer buffer
      (anvillm-tasks-mode)
      (anvillm--refresh-tasks))
    (switch-to-buffer buffer)))

(defun anvillm-tasks-refresh ()
  "Refresh the tasks list."
  (interactive)
  (anvillm--refresh-tasks))

(defun anvillm-tasks-new ()
  "Create a new bead."
  (interactive)
  (let ((title (read-string "Title: "))
        (description (read-string "Description (optional): ")))
    (when (> (length title) 0)
      (condition-case err
          (let ((cmd (if (> (length description) 0)
                        (format "new '%s' '%s'" title description)
                      (format "new '%s'" title))))
            (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl") cmd)
            (message "Created bead: %s" title)
            (anvillm--refresh-tasks))
        (error
         (message "Failed to create bead: %s" (error-message-string err)))))))

(defun anvillm-tasks-new-subtask ()
  "Create a new subtask for the selected bead."
  (interactive)
  (if-let ((parent-id (anvillm--get-selected-bead)))
      (let ((title (read-string "Subtask title: "))
            (description (read-string "Description (optional): ")))
        (when (> (length title) 0)
          (condition-case err
              (let ((cmd (if (> (length description) 0)
                            (format "new '%s' '%s' %s" title description parent-id)
                          (format "new '%s' '' %s" title parent-id))))
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl") cmd)
                (message "Created subtask of %s: %s" parent-id title)
                (anvillm--refresh-tasks))
            (error
             (message "Failed to create subtask: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm--get-selected-bead ()
  "Get the ID of the currently selected bead."
  (tabulated-list-get-id))

(defun anvillm-tasks-claim ()
  "Claim the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (condition-case err
          (progn
            (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                              (format "claim %s" bead-id))
            (message "Claimed bead %s" bead-id)
            (anvillm--refresh-tasks))
        (error
         (message "Failed to claim bead: %s" (error-message-string err))))
    (message "No bead selected")))

(defun anvillm-tasks-complete ()
  "Complete the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (condition-case err
          (progn
            (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                              (format "complete %s" bead-id))
            (message "Completed bead %s" bead-id)
            (anvillm--refresh-tasks))
        (error
         (message "Failed to complete bead: %s" (error-message-string err))))
    (message "No bead selected")))

(defun anvillm-tasks-fail ()
  "Fail the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (let ((reason (read-string "Reason: ")))
        (condition-case err
            (progn
              (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                (format "fail %s '%s'" bead-id reason))
              (message "Failed bead %s" bead-id)
              (anvillm--refresh-tasks))
          (error
           (message "Failed to fail bead: %s" (error-message-string err)))))
    (message "No bead selected")))

(defun anvillm-tasks-view ()
  "View details of the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (condition-case err
          (let* ((json-object-type 'plist)
                 (json-array-type 'list)
                 (bead-json (anvillm--9p-read 
                            (concat anvillm-agent-path "/beads/" bead-id "/json")))
                 (bead (json-read-from-string bead-json))
                 (buffer (get-buffer-create (format "*Bead: %s*" bead-id))))
            (with-current-buffer buffer
              (erase-buffer)
              (insert (format "ID: %s\n" (plist-get bead :id)))
              (insert (format "Title: %s\n" (plist-get bead :title)))
              (insert (format "Status: %s\n" (plist-get bead :status)))
              (insert (format "Priority: %s\n" (or (plist-get bead :priority) "N/A")))
              (insert (format "Assignee: %s\n" (or (plist-get bead :assignee) "N/A")))
              (insert (format "Type: %s\n" (or (plist-get bead :issue_type) "N/A")))
              (insert (format "Created: %s\n" (plist-get bead :created_at)))
              (insert (format "Updated: %s\n" (plist-get bead :updated_at)))
              (when-let ((desc (plist-get bead :description)))
                (insert (format "\nDescription:\n%s\n" desc)))
              (when-let ((blockers (plist-get bead :blockers)))
                (insert (format "\nBlockers (%d):\n" (length blockers)))
                (dolist (blocker blockers)
                  (insert (format "  - %s\n" blocker))))
              (special-mode))
            (pop-to-buffer buffer))
        (error
         (message "Failed to view bead: %s" (error-message-string err))))
    (message "No bead selected")))

(defun anvillm-tasks-add-dependency ()
  "Add a dependency to the selected bead."
  (interactive)
  (if-let ((child-id (anvillm--get-selected-bead)))
      (let ((parent-id (read-string "Parent bead ID: ")))
        (when (> (length parent-id) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                  (format "dep %s %s" child-id parent-id))
                (message "Added dependency: %s blocks %s" parent-id child-id)
                (anvillm--refresh-tasks))
            (error
             (message "Failed to add dependency: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm-tasks-remove-dependency ()
  "Remove a dependency from the selected bead."
  (interactive)
  (if-let ((child-id (anvillm--get-selected-bead)))
      (let ((parent-id (read-string "Parent bead ID to remove: ")))
        (when (> (length parent-id) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                  (format "undep %s %s" child-id parent-id))
                (message "Removed dependency: %s no longer blocks %s" parent-id child-id)
                (anvillm--refresh-tasks))
            (error
             (message "Failed to remove dependency: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm-tasks-add-label ()
  "Add a label to the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (let ((label (read-string "Label: ")))
        (when (> (length label) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                  (format "label %s '%s'" bead-id label))
                (message "Added label '%s' to %s" label bead-id)
                (anvillm--refresh-tasks))
            (error
             (message "Failed to add label: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm-tasks-remove-label ()
  "Remove a label from the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (let ((label (read-string "Label to remove: ")))
        (when (> (length label) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                  (format "unlabel %s '%s'" bead-id label))
                (message "Removed label '%s' from %s" label bead-id)
                (anvillm--refresh-tasks))
            (error
             (message "Failed to remove label: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm-tasks-assign-to-agent ()
  "Assign the selected bead to an agent to work on."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (let* ((sessions (anvillm--list-sessions))
             (agent-ids (mapcar #'car sessions))
             (agent-id (completing-read "Agent ID: " agent-ids nil nil)))
        (when (> (length agent-id) 0)
          (condition-case err
              (let ((msg (json-encode `((to . ,agent-id)
                                       (type . "PROMPT_REQUEST")
                                       (subject . "User prompt")
                                       (body . ,(format "Work on bead %s" bead-id))))))
                (anvillm--9p-write (concat anvillm-agent-path "/user/mail") msg)
                (message "Assigned bead %s to %s" bead-id (substring agent-id 0 (min 8 (length agent-id)))))
            (error
             (message "Failed to assign bead: %s" (error-message-string err))))))
    (message "No bead selected")))


(defun anvillm-tasks-comment ()
  "Add a comment to the selected bead."
  (interactive)
  (if-let ((bead-id (anvillm--get-selected-bead)))
      (let ((comment (read-string "Comment: ")))
        (when (> (length comment) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/beads/ctl")
                                  (format "comment %s '%s'" bead-id comment))
                (message "Added comment to %s" bead-id))
            (error
             (message "Failed to add comment: %s" (error-message-string err))))))
    (message "No bead selected")))

(defun anvillm-tasks-help ()
  "Show help for AnviLLM tasks mode."
  (interactive)
  (with-help-window "*AnviLLM Tasks Help*"
    (princ "AnviLLM Tasks - Beads Management

Keybindings:
SPC - Create new bead
s - Create new subtask for selected bead
c - Claim selected bead
C - Complete selected bead
f - Fail selected bead (with reason)
v - View bead details
d - Add dependency (parent blocks child)
D - Remove dependency
l - Add label
L - Remove label
m - Add comment
r, g - Refresh tasks list
q - Quit
? - This help

Navigation:
n, C-n - Next line
p, C-p - Previous line

Bead Status:
open - Not yet started
in_progress - Currently being worked on
closed - Completed

")))

;;; Major Mode

(defvar anvillm-mode-map
  (let ((map (make-sparse-keymap)))
    (set-keymap-parent map tabulated-list-mode-map)
    (define-key map (kbd "s") #'anvillm-start-session)
    (define-key map (kbd "T") #'anvillm-stop-session)
    (define-key map (kbd "R") #'anvillm-restart-session)
    (define-key map (kbd "K") #'anvillm-kill-session)
    (define-key map (kbd "A") #'anvillm-set-alias)
    (define-key map (kbd "a") #'anvillm-attach-session)
    (define-key map (kbd "w") #'anvillm-assign-bead)
    (define-key map (kbd "SPC") #'anvillm-compose-prompt)
    (define-key map (kbd "M-SPC") #'anvillm-send-prompt)
    (define-key map (kbd "p") #'previous-line)
    (define-key map (kbd "n") #'next-line)
    (define-key map (kbd "l") #'anvillm-view-log)
    (define-key map (kbd "c") #'anvillm-edit-context)
    (define-key map (kbd "i") #'anvillm-inbox)
    (define-key map (kbd "C") #'anvillm-archive)
    (define-key map (kbd "t") #'anvillm-tasks)
    (define-key map (kbd "r") #'anvillm-refresh)
    (define-key map (kbd "g") #'anvillm-refresh)
    (define-key map (kbd "d") #'anvillm-daemon-status)
    (define-key map (kbd "q") #'quit-window)
    (define-key map (kbd "?") #'anvillm-help)
    map)
  "Keymap for AnviLLM mode.")

(define-derived-mode anvillm-mode tabulated-list-mode "AnviLLM"
  "Major mode for managing AnviLLM sessions.

\\{anvillm-mode-map}"
  (setq tabulated-list-format [("ID" 10 t)
                                ("Alias" 15 t)
                                ("Backend" 12 t)
                                ("State" 10 t)
                                ("PID" 8 t)
                                ("Cwd" 0 t)])
  (setq tabulated-list-padding 2)
  (setq tabulated-list-sort-key (cons "ID" nil))
  (tabulated-list-init-header)

  ;; Set up auto-refresh timer
  (when anvillm-refresh-timer
    (cancel-timer anvillm-refresh-timer))
  (setq anvillm-refresh-timer
        (run-at-time anvillm-refresh-interval anvillm-refresh-interval
                     (lambda ()
                       (when (and (buffer-live-p (get-buffer "*AnviLLM*"))
                                  (eq (buffer-local-value 'major-mode (get-buffer "*AnviLLM*")) 'anvillm-mode))
                         (with-current-buffer "*AnviLLM*"
                           (anvillm--refresh-sessions))))))

  ;; Clean up timer when buffer is killed
  (add-hook 'kill-buffer-hook
            (lambda ()
              (when anvillm-refresh-timer
                (cancel-timer anvillm-refresh-timer)
                (setq anvillm-refresh-timer nil)))
            nil t))

;;;###autoload
(defun anvillm ()
  "Open the AnviLLM session manager."
  (interactive)
  (let ((buffer (get-buffer-create "*AnviLLM*")))
    (with-current-buffer buffer
      (anvillm-mode)
      (anvillm--refresh-sessions))
    (switch-to-buffer buffer)))

(defcustom anvillm-terminal-command
  (or (getenv "ANVILLM_TERMINAL") "foot")
  "Terminal emulator command for attaching to tmux sessions."
  :type 'string
  :group 'anvillm)

(defun anvillm-attach-session ()
  "Attach to the tmux session for the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (condition-case err
          (let ((tmux-target (string-trim (anvillm--9p-read (concat anvillm-agent-path "/" session-id "/tmux")))))
            (if (string-empty-p tmux-target)
                (message "Session does not support attach")
              (start-process
               (format "tmux-attach-%s" session-id)
               nil
               anvillm-terminal-command
               "-e" "tmux" "attach" "-t" tmux-target)
              (message "Attached to tmux session %s" tmux-target)))
        (error
         (message "Failed to attach to session: %s" (error-message-string err))))
    (message "No session selected")))

(defvar-local anvillm--log-session-id nil
  "Session ID for the current log buffer.")

(defvar-local anvillm--log-process nil
  "Process streaming log data for the current buffer.")

(defvar anvillm-log-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd "r") #'anvillm-log-refresh)
    (define-key map (kbd "g") #'anvillm-log-refresh)
    (define-key map (kbd "q") #'anvillm-log-quit)
    map)
  "Keymap for AnviLLM log mode.")

(defun anvillm-log-quit ()
  "Quit the log window and kill the streaming process."
  (interactive)
  ;; Kill the streaming process
  (when (and anvillm--log-process
            (process-live-p anvillm--log-process))
    (delete-process anvillm--log-process)
    (setq anvillm--log-process nil))
  ;; Quit the window
  (quit-window t))

(define-derived-mode anvillm-log-mode special-mode "AnviLLM-Log"
  "Major mode for viewing AnviLLM session logs.

\\{anvillm-log-mode-map}"
  (setq buffer-read-only t)
  (setq header-line-format
        (substitute-command-keys
         "AnviLLM Log (streaming). Refresh: \\[anvillm-log-refresh] | Quit: \\[quit-window]"))
  ;; Kill the streaming process when buffer is killed
  (add-hook 'kill-buffer-hook
            (lambda ()
              (when (and anvillm--log-process
                        (process-live-p anvillm--log-process))
                (delete-process anvillm--log-process)))
            nil t))

(defun anvillm-view-log ()
  "View the centralized audit log."
  (interactive)
  (let* ((buffer-name "*AnviLLM Audit Log*")
         (buffer (get-buffer-create buffer-name)))
    (with-current-buffer buffer
      (anvillm-log-mode)
      (setq anvillm--log-session-id nil) ; Not session-specific anymore
      (anvillm--refresh-log-buffer))
    (pop-to-buffer buffer)))

(defun anvillm--refresh-log-buffer ()
  "Refresh the log content in the current buffer by starting a streaming read."
  
  ;; Kill existing process if any
  (when (and anvillm--log-process
            (process-live-p anvillm--log-process))
    (delete-process anvillm--log-process))
  
  (let ((inhibit-read-only t)
        (log-buffer (current-buffer)))
    (erase-buffer)
    (insert "Loading audit log...\n")
    
    ;; Start streaming process for centralized audit log
    (let* ((path (concat anvillm-agent-path "/audit"))
           (proc (start-process "9p-log-stream" log-buffer anvillm-9p-command "read" path)))
      
      (setq anvillm--log-process proc)
      
      (set-process-filter
       proc
       (lambda (process output)
         (when (buffer-live-p log-buffer)
           (with-current-buffer log-buffer
             (let ((inhibit-read-only t)
                   (at-end (= (point) (point-max))))
               ;; Clear "Loading audit log..." on first output
               (when (save-excursion
                       (goto-char (point-min))
                       (looking-at "Loading audit log..."))
                 (erase-buffer))
               ;; Insert new output
               (goto-char (point-max))
               (insert output)
               ;; Auto-scroll if we were at the end
               (when at-end
                 (goto-char (point-max))))))))
      
      (set-process-sentinel
       proc
       (lambda (process event)
         (when (buffer-live-p log-buffer)
           (with-current-buffer log-buffer
             (let ((inhibit-read-only t))
               (when (and (= (point-min) (point-max))
                         (not (string-match-p "^run" event)))
                 (insert "No audit log entries yet.\n"))))))))))

(defun anvillm-log-refresh ()
  "Refresh the log content."
  (interactive)
  (anvillm--refresh-log-buffer)
  (message "Log refreshed"))

(defvar-local anvillm--prompt-session-id nil
  "Session ID for the current prompt buffer.")

(defvar-local anvillm--reply-subject nil
  "Subject line for reply messages.")

(defvar anvillm-prompt-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd "C-c C-c") #'anvillm-prompt-send)
    (define-key map (kbd "C-c C-k") #'anvillm-prompt-abort)
    map)
  "Keymap for AnviLLM prompt composition mode.")

(define-derived-mode anvillm-prompt-mode text-mode "AnviLLM-Prompt"
  "Major mode for composing prompts to send to AnviLLM sessions.

\\{anvillm-prompt-mode-map}"
  (setq header-line-format
        (substitute-command-keys
         "Compose prompt for AnviLLM session. Finish: \\[anvillm-prompt-send] | Abort: \\[anvillm-prompt-abort]")))

(defun anvillm-compose-prompt ()
  "Open a buffer to compose a prompt for the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let* ((session-info (anvillm--get-session-info session-id))
             (display-name (or (plist-get session-info :alias) 
                              (substring session-id 0 (min 8 (length session-id)))))
             (buffer-name (format "*AnviLLM Prompt: %s*" display-name))
             (buffer (get-buffer-create buffer-name)))
        (with-current-buffer buffer
          (anvillm-prompt-mode)
          (erase-buffer)
          (setq anvillm--prompt-session-id session-id)
          (insert (format ";; Prompt for session: %s\n" display-name))
          (insert (format ";; Backend: %s\n" (plist-get session-info :backend)))
          (insert (format ";; State: %s\n\n" (plist-get session-info :state)))
          (insert ";; Type your prompt below (comments will be stripped).\n")
          (insert ";; Press C-c C-c to send, C-c C-k to abort.\n\n"))
        (pop-to-buffer buffer)
        (goto-char (point-max)))
    (message "No session selected")))

(defun anvillm--get-session-info (session-id)
  "Get session information for SESSION-ID as a plist."
  (let ((backend (condition-case nil
                     (string-trim (anvillm--9p-read 
                                  (concat anvillm-agent-path "/" session-id "/backend")))
                   (error "")))
        (state (condition-case nil
                   (string-trim (anvillm--9p-read 
                                (concat anvillm-agent-path "/" session-id "/state")))
                 (error "")))
        (alias (condition-case nil
                   (string-trim (anvillm--9p-read 
                                (concat anvillm-agent-path "/" session-id "/alias")))
                 (error ""))))
    (list :backend backend
          :state state
          :alias (if (string= alias "") nil alias))))

(defun anvillm-send-prompt ()
  "Send a prompt to the selected session (using minibuffer)."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let* ((prompt (read-string "Prompt: "))
             (wrapped-prompt (anvillm--wrap-bead-id prompt)))
        (when (> (length prompt) 0)
          (condition-case err
              (let ((msg (json-encode `((to . ,session-id)
                                       (type . "PROMPT_REQUEST")
                                       (subject . "User prompt")
                                       (body . ,wrapped-prompt)))))
                (anvillm--9p-write (concat anvillm-agent-path "/user/mail") msg)
                (message "Sent prompt to %s" (substring session-id 0 (min 8 (length session-id)))))
            (error
             (message "Failed to send prompt: %s" (error-message-string err))))))
    (message "No session selected")))

(defun anvillm-prompt-abort ()
  "Abort prompt composition and close the buffer."
  (interactive)
  (when (yes-or-no-p "Abort prompt composition? ")
    (quit-window t)))

(defun anvillm--is-bead-id (text)
  "Check if TEXT matches bead ID pattern (e.g., bd-5xz or bd-5xz.1)."
  (string-match-p "^[a-zA-Z]+-[a-z0-9]+\\(\\.[0-9]+\\)*$" text))

(defun anvillm--wrap-bead-id (prompt)
  "If PROMPT is a bead ID, wrap it with execution instructions."
  (let ((trimmed (string-trim prompt)))
    (if (anvillm--is-bead-id trimmed)
        (format "Load the beads skill, and work on bead %s." trimmed)
      prompt)))

(defun anvillm--extract-prompt-text ()
  "Extract prompt text from buffer, stripping comment lines."
  (let ((lines (split-string (buffer-string) "\n")))
    (string-join
     (delq nil
           (mapcar (lambda (line)
                     (unless (string-prefix-p ";;" (string-trim line))
                       line))
                   lines))
     "\n")))


(defun anvillm-prompt-send ()
  "Send the composed prompt to the session and close the buffer."
  (interactive)
  (unless anvillm--prompt-session-id
    (error "No session ID associated with this buffer"))
  (let* ((prompt (anvillm--extract-prompt-text))
         (wrapped-prompt (anvillm--wrap-bead-id prompt))
         (subject (or anvillm--reply-subject "User prompt")))
    (if (string-empty-p (string-trim prompt))
        (message "Empty prompt, not sending")
      (condition-case err
          (let ((msg (json-encode `((to . ,anvillm--prompt-session-id)
                                   (type . "PROMPT_REQUEST")
                                   (subject . ,subject)
                                   (body . ,wrapped-prompt)))))
            (anvillm--9p-write (concat anvillm-agent-path "/user/mail") msg)
            (message "Sent prompt to %s" 
                    (substring anvillm--prompt-session-id 0 
                              (min 8 (length anvillm--prompt-session-id))))
            (quit-window t))
        (error
         (message "Failed to send prompt: %s" (error-message-string err)))))))

(provide 'anvillm)

;;; anvillm.el ends here
