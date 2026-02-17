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
;;   p - Send prompt to selected session
;;   t - Stop selected session
;;   R - Restart selected session
;;   K - Kill selected session
;;   a - Set alias for selected session
;;   r - Refresh session list
;;   g - Refresh session list (standard Emacs binding)
;;   d - Daemon status
;;   q - Quit
;;   ? - Help

;;; Code:

(require 'tabulated-list)

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
Format: id alias state pid cwd (whitespace-separated; often tabs)."
  (when (string-match
         "^\\([^[:space:]]+\\)\\s-+\\([^[:space:]]+\\)\\s-+\\([^[:space:]]+\\)\\s-+\\([0-9]+\\)\\s-+\\(.+\\)$"
         line)
    (let ((id (match-string 1 line))
          (alias (match-string 2 line))
          (state (match-string 3 line))
          (pid (match-string 4 line))
          (cwd (match-string 5 line)))
      (when (string= alias "-") (setq alias ""))
      (let ((backend (condition-case nil
                         (string-trim (anvillm--9p-read (concat anvillm-agent-path "/" id "/backend")))
                       (error ""))))
        (list id (vector
                  (substring id 0 (min 8 (length id)))
                  alias
                  backend
                  (propertize state 'face (anvillm--state-face state))
                  pid
                  cwd))))))

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

(defun anvillm-send-prompt-minibuffer ()
  "Send a prompt to the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let ((prompt (read-string "Prompt: ")))
        (when (> (length prompt) 0)
          (condition-case err
              (progn
                ;; Create message JSON for user outbox
                (let* ((timestamp (format-time-string "%s"))
                       (filename (format "msg-%s.json" timestamp))
                       (path (concat anvillm-agent-path "/user/outbox/" filename))
                       (msg (json-encode `((to . ,session-id)
                                         (type . "PROMPT")
                                         (subject . "User prompt")
                                         (body . ,prompt)))))
                  (anvillm--9p-write path msg))
                (message "Sent prompt to %s" (substring session-id 0 (min 8 (length session-id)))))
            (error
             (message "Failed to send prompt: %s" (error-message-string err))))))
    (message "No session selected")))

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
p - Compose prompt in buffer (C-c C-c to send, C-c C-k to abort)
l - View session log (press 'r' to refresh, 'q' to close)
t - Stop selected session
R - Restart selected session
K - Kill selected session
a - Set alias for selected session
A - Attach to tmux session in terminal
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

9P Filesystem:
All operations read/write the 9P filesystem at $NAMESPACE/agent

Backends:
- claude (Claude Code CLI)
- kiro-cli (Kiro CLI)

")))


;;; Major Mode

(defvar anvillm-mode-map
  (let ((map (make-sparse-keymap)))
    (set-keymap-parent map tabulated-list-mode-map)
    (define-key map (kbd "s") #'anvillm-start-session)
    (define-key map (kbd "P") #'anvillm-send-prompt-minibuffer)
    (define-key map (kbd "t") #'anvillm-stop-session)
    (define-key map (kbd "R") #'anvillm-restart-session)
    (define-key map (kbd "K") #'anvillm-kill-session)
    (define-key map (kbd "A") #'anvillm-set-alias)
    (define-key map (kbd "a") #'anvillm-attach-session)
    (define-key map (kbd "p") #'anvillm-compose-prompt)
    (define-key map (kbd "l") #'anvillm-view-log)
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
          (let* ((backend (string-trim (anvillm--9p-read (concat anvillm-agent-path "/" session-id "/backend"))))
                 (tmux-session (format "anvillm-%s" backend))
                 (tmux-window session-id)
                 (tmux-target (format "%s:%s" tmux-session tmux-window)))
            (start-process
             (format "tmux-attach-%s" session-id)
             nil
             anvillm-terminal-command
             "-e" "tmux" "attach" "-t" tmux-target)
            (message "Attached to tmux session %s" tmux-target))
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
  "View the log for the selected session."
  (interactive)
  (if-let ((session-id (anvillm--get-selected-session)))
      (let* ((session-info (anvillm--get-session-info session-id))
             (display-name (or (plist-get session-info :alias)
                              (substring session-id 0 (min 8 (length session-id)))))
             (buffer-name (format "*AnviLLM Log: %s*" display-name))
             (buffer (get-buffer-create buffer-name)))
        (with-current-buffer buffer
          (anvillm-log-mode)
          (setq anvillm--log-session-id session-id)
          (anvillm--refresh-log-buffer))
        (pop-to-buffer buffer))
    (message "No session selected")))

(defun anvillm--refresh-log-buffer ()
  "Refresh the log content in the current buffer by starting a streaming read."
  (unless anvillm--log-session-id
    (error "No session ID associated with this buffer"))
  
  ;; Kill existing process if any
  (when (and anvillm--log-process
            (process-live-p anvillm--log-process))
    (delete-process anvillm--log-process))
  
  (let ((inhibit-read-only t)
        (log-buffer (current-buffer)))
    (erase-buffer)
    (insert "Loading log...\n")
    
    ;; Start streaming process
    (let* ((path (concat anvillm-agent-path "/" anvillm--log-session-id "/log"))
           (proc (start-process "9p-log-stream" log-buffer anvillm-9p-command "read" path)))
      
      (setq anvillm--log-process proc)
      
      (set-process-filter
       proc
       (lambda (process output)
         (when (buffer-live-p log-buffer)
           (with-current-buffer log-buffer
             (let ((inhibit-read-only t)
                   (at-end (= (point) (point-max))))
               ;; Clear "Loading log..." on first output
               (when (save-excursion
                       (goto-char (point-min))
                       (looking-at "Loading log..."))
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
                 (insert "No log output yet.\n"))))))))))

(defun anvillm-log-refresh ()
  "Refresh the log content."
  (interactive)
  (anvillm--refresh-log-buffer)
  (message "Log refreshed"))

(defvar-local anvillm--prompt-session-id nil
  "Session ID for the current prompt buffer.")

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
      (let ((prompt (read-string "Prompt: ")))
        (when (> (length prompt) 0)
          (condition-case err
              (progn
                (anvillm--9p-write (concat anvillm-agent-path "/" session-id "/in") prompt)
                (message "Sent prompt to %s" (substring session-id 0 (min 8 (length session-id)))))
            (error
             (message "Failed to send prompt: %s" (error-message-string err))))))
    (message "No session selected")))

(defun anvillm-prompt-abort ()
  "Abort prompt composition and close the buffer."
  (interactive)
  (when (yes-or-no-p "Abort prompt composition? ")
    (quit-window t)))

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
  (let ((prompt (anvillm--extract-prompt-text)))
    (if (string-empty-p (string-trim prompt))
        (message "Empty prompt, not sending")
      (condition-case err
          (progn
            (anvillm--9p-write 
             (concat anvillm-agent-path "/" anvillm--prompt-session-id "/in")
             prompt)
            (message "Sent prompt to %s" 
                    (substring anvillm--prompt-session-id 0 
                              (min 8 (length anvillm--prompt-session-id))))
            (quit-window t))
        (error
         (message "Failed to send prompt: %s" (error-message-string err)))))))

(provide 'anvillm)

;;; anvillm.el ends here
