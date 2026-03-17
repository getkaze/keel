// keel Dashboard — Client-side JavaScript
// Handles: toasts, modals, SSE rendering, terminal (lazy-loaded)

// PWA: register service worker
if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(function() {});
}

// Version check: show update indicator if new version available
var _versionData = null;

fetch('/api/version').then(function(r) { return r.json(); }).then(function(data) {
    _versionData = data;
    if (data.available) {
        var el = document.getElementById('update-indicator');
        var ver = document.getElementById('update-version');
        if (el && ver) {
            ver.textContent = data.latest;
            el.style.display = '';
        }
    }
}).catch(function() {});

// Update modal
function openUpdateModal() {
    var modal = document.getElementById('update-modal');
    if (!modal || !_versionData) return;

    document.getElementById('update-modal-current').textContent = _versionData.current;
    document.getElementById('update-modal-latest').textContent = _versionData.latest;

    // Render changelog
    var changelogEl = document.getElementById('update-modal-changelog');
    if (_versionData.changelog) {
        changelogEl.innerHTML = renderMarkdown(_versionData.changelog);
    } else {
        changelogEl.innerHTML = '<p class="text-muted">No release notes available.</p>';
    }

    // Reset state
    document.getElementById('update-modal-progress').style.display = 'none';
    document.getElementById('update-modal-progress').innerHTML = '';
    document.getElementById('update-modal-btn').disabled = false;
    document.getElementById('update-modal-btn').style.display = '';
    document.getElementById('update-modal-cancel').style.display = '';

    modal.showModal();
}

function performUpdate() {
    var btn = document.getElementById('update-modal-btn');
    var progress = document.getElementById('update-modal-progress');
    var changelog = document.getElementById('update-modal-changelog');
    var cancelBtn = document.getElementById('update-modal-cancel');

    btn.disabled = true;
    btn.innerHTML = '<span class="spinner-sm"></span> Updating...';
    cancelBtn.style.display = 'none';
    changelog.style.display = 'none';
    progress.style.display = '';

    fetchSSEUpdate(progress, btn, cancelBtn);
}

function fetchSSEUpdate(progress, btn, cancelBtn) {
    fetch('/api/update', { method: 'POST' }).then(function(response) {
        var reader = response.body.getReader();
        var decoder = new TextDecoder();
        var buffer = '';

        function read() {
            reader.read().then(function(result) {
                if (result.done) return;
                buffer += decoder.decode(result.value, { stream: true });
                var lines = buffer.split('\n');
                buffer = lines.pop();
                for (var i = 0; i < lines.length; i++) {
                    processSSELine(lines[i], progress, btn, cancelBtn);
                }
                read();
            });
        }
        read();
    }).catch(function(err) {
        var line = document.createElement('div');
        line.className = 'text-error font-semibold';
        line.textContent = 'Connection failed: ' + err.message;
        progress.appendChild(line);
        btn.disabled = false;
        btn.innerHTML = '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24" style="margin-right:4px"><use href="/static/lucide-sprite.svg#download"></use></svg> Retry';
        cancelBtn.style.display = '';
    });
}

var _sseEventType = '';

function processSSELine(line, progress, btn, cancelBtn) {
    if (line.indexOf('event: ') === 0) {
        _sseEventType = line.substring(7).trim();
        return;
    }
    if (line.indexOf('data: ') !== 0) {
        if (line === '') _sseEventType = '';
        return;
    }
    var data = line.substring(6);
    var eventType = _sseEventType;
    _sseEventType = '';

    var el = document.createElement('div');

    if (eventType === 'done') {
        el.className = 'text-success font-semibold';
        el.textContent = data;
        progress.appendChild(el);
        btn.style.display = 'none';

        var reloadLine = document.createElement('div');
        reloadLine.className = 'text-muted mt-1';
        reloadLine.textContent = 'Restarting keel... reloading in a few seconds.';
        progress.appendChild(reloadLine);
        setTimeout(function() { waitForRestart(); }, 2000);
    } else if (eventType === 'error') {
        el.className = 'text-error font-semibold';
        el.textContent = data;
        progress.appendChild(el);
        btn.disabled = false;
        btn.innerHTML = '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24" style="margin-right:4px"><use href="/static/lucide-sprite.svg#download"></use></svg> Retry';
        cancelBtn.style.display = '';
    } else {
        el.textContent = data;
        progress.appendChild(el);
    }
    progress.scrollTop = progress.scrollHeight;
}

function waitForRestart() {
    var attempts = 0;
    var maxAttempts = 30;
    function tryReload() {
        attempts++;
        fetch('/api/health').then(function(r) {
            if (r.ok) {
                window.location.reload();
            } else {
                retry();
            }
        }).catch(function() {
            retry();
        });
    }
    function retry() {
        if (attempts < maxAttempts) {
            setTimeout(tryReload, 1000);
        } else {
            var progress = document.getElementById('update-modal-progress');
            if (progress) {
                var el = document.createElement('div');
                el.className = 'text-warning';
                el.textContent = 'Could not reconnect. Please reload the page manually.';
                progress.appendChild(el);
            }
        }
    }
    tryReload();
}

// Changelog markdown renderer with category icons
var changelogIcons = {
    "added":   '<svg class="changelog-icon changelog-icon-added" viewBox="0 0 24 24"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
    "changed": '<svg class="changelog-icon changelog-icon-changed" viewBox="0 0 24 24"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
    "fixed":   '<svg class="changelog-icon changelog-icon-fixed" viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>',
    "removed": '<svg class="changelog-icon changelog-icon-removed" viewBox="0 0 24 24"><line x1="5" y1="12" x2="19" y2="12"/></svg>',
    "what's new": '<svg class="changelog-icon changelog-icon-added" viewBox="0 0 24 24"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>'
};

function renderMarkdown(text) {
    var lines = text.split('\n');
    var html = '';
    var inList = false;
    var inSection = false;

    for (var i = 0; i < lines.length; i++) {
        var line = lines[i];

        if (/^#{2,3}\s+/.test(line)) {
            if (inList) { html += '</ul>'; inList = false; }
            if (inSection) { html += '</div>'; inSection = false; }

            var heading = line.replace(/^#{2,3}\s+/, '').trim();
            var key = heading.toLowerCase();
            var icon = changelogIcons[key] || '';

            html += '<div class="changelog-section">';
            html += '<h4 class="changelog-heading">' + icon + escapeHtml(heading) + '</h4>';
            inSection = true;
        } else if (/^[-*]\s+/.test(line)) {
            if (!inList) { html += '<ul class="changelog-list">'; inList = true; }
            var content = line.replace(/^[-*]\s+/, '');
            // Bold text between ** **
            var rendered = escapeHtml(content).replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
            html += '<li>' + rendered + '</li>';
        } else if (line.trim() === '') {
            if (inList) { html += '</ul>'; inList = false; }
        } else if (line.trim()) {
            if (inList) { html += '</ul>'; inList = false; }
            html += '<p class="changelog-text">' + escapeHtml(line) + '</p>';
        }
    }
    if (inList) html += '</ul>';
    if (inSection) html += '</div>';

    if (!html.trim()) {
        html = '<p class="text-muted">No release notes available.</p>';
    }
    return html;
}

// AnsiUp instance for ANSI color rendering
let ansiUp = null;

function getAnsiUp() {
    if (!ansiUp && typeof AnsiUp !== 'undefined') {
        ansiUp = new AnsiUp();
        ansiUp.use_classes = true;
    }
    return ansiUp;
}

// Toast system
function showToast(message, type) {
    type = type || 'info';
    var container = document.getElementById('toast-container');
    if (!container) return;

    var classes = {
        success: 'toast-success',
        error: 'toast-error',
        warning: 'toast-warning',
        info: 'toast-info'
    };
    var durations = { success: 4000, error: 8000, warning: 6000, info: 4000 };

    var el = document.createElement('div');
    el.className = 'toast ' + (classes[type] || 'toast-info');
    el.innerHTML = '<span>' + escapeHtml(message) + '</span>';

    container.appendChild(el);

    // Limit to 3 visible
    while (container.children.length > 3) {
        container.removeChild(container.firstElementChild);
    }

    setTimeout(function() {
        el.style.opacity = '0';
        setTimeout(function() { el.remove(); }, 300);
    }, durations[type] || 4000);
}

function escapeHtml(text) {
    var div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Sanitize AnsiUp HTML output — only allow <span> tags with class attributes.
// Prevents XSS from malicious content injected via container logs.
function safeAnsiHtml(converter, text) {
    if (!converter) return escapeHtml(text);
    var html = converter.ansi_to_html(text);
    var tmp = document.createElement('div');
    tmp.innerHTML = html;
    sanitizeNode(tmp);
    return tmp.innerHTML;
}

function sanitizeNode(node) {
    var children = Array.from(node.childNodes);
    for (var i = 0; i < children.length; i++) {
        var child = children[i];
        if (child.nodeType === Node.ELEMENT_NODE) {
            if (child.tagName !== 'SPAN') {
                // Replace disallowed element with its text content
                child.replaceWith(document.createTextNode(child.textContent));
            } else {
                // Remove all attributes except class
                var attrs = Array.from(child.attributes);
                for (var j = 0; j < attrs.length; j++) {
                    if (attrs[j].name !== 'class') child.removeAttribute(attrs[j].name);
                }
                sanitizeNode(child);
            }
        }
    }
}

// Confirmation modal helper
function confirmAction(title, message, confirmText, confirmClass, onConfirm) {
    var modal = document.getElementById('confirm-modal');
    if (!modal) return;

    document.getElementById('confirm-title').textContent = title;
    document.getElementById('confirm-message').textContent = message;

    var btn = document.getElementById('confirm-btn');
    btn.textContent = confirmText;
    btn.onclick = function() {
        modal.close();
        onConfirm();
    };

    modal.showModal();
}

// SSE stream handler for operation progress
function startSSE(url, opts) {
    opts = opts || {};
    var panel = document.getElementById('operation-output');
    if (panel) {
        panel.innerHTML = '';
        document.getElementById('operation-panel').classList.remove('hidden');
    }

    var source = new EventSource(url);

    source.onmessage = function(e) {
        if (!panel) return;
        var converter = getAnsiUp();
        var line = document.createElement('div');
        line.innerHTML = safeAnsiHtml(converter, e.data);
        panel.appendChild(line);
        panel.scrollTop = panel.scrollHeight;
    };

    source.addEventListener('done', function(e) {
        source.close();
        if (panel) {
            var banner = document.createElement('div');
            banner.className = 'text-success font-semibold mt-2';
            banner.textContent = e.data || 'Operation completed successfully';
            panel.appendChild(banner);
        }
        showToast(opts.successMessage || 'Operation completed', 'success');
        htmx.trigger(document.body, 'refreshServices');
    });

    source.addEventListener('error', function(e) {
        source.close();
        if (panel) {
            var banner = document.createElement('div');
            banner.className = 'text-error font-semibold mt-2';
            banner.textContent = e.data || 'Operation failed';
            panel.appendChild(banner);
        }
        showToast(opts.errorMessage || 'Operation failed', 'error');
    });

    return source;
}

// Keyboard shortcuts
document.addEventListener('keydown', function(e) {
    // Ctrl+` — toggle terminal
    if (e.ctrlKey && e.key === '`') {
        e.preventDefault();
        document.dispatchEvent(new CustomEvent('toggle-terminal'));
    }

    // Escape — close modals/panels
    if (e.key === 'Escape') {
        var modal = document.getElementById('confirm-modal');
        if (modal && modal.open) {
            modal.close();
        }
    }
});

// Navigation: update active state on nav items
function setActiveNav(page) {
    document.querySelectorAll('[data-page]').forEach(function(el) {
        el.classList.toggle('active', el.getAttribute('data-page') === page);
    });
}

document.addEventListener('click', function(e) {
    var navItem = e.target.closest('[data-page]');
    if (!navItem) return;
    setActiveNav(navItem.getAttribute('data-page'));
});

// Open terminal panel connected to a container via docker exec
function connectToContainer(name) {
    document.dispatchEvent(new CustomEvent('connect-container', { detail: { name: name } }));
}

// Navigate to logs page with a specific service pre-selected
function navigateToLogs(serviceName) {
    htmx.ajax('GET', '/partials/logs', {target: '#main-content', swap: 'innerHTML'}).then(function() {
        // Wait for Alpine to initialize the log-viewer component, then dispatch event
        setTimeout(function() {
            document.dispatchEvent(new CustomEvent('open-logs', { detail: { service: serviceName } }));
        }, 100);
    });
    history.pushState(null, '', '/logs');
    setActiveNav('/logs');
}

// HTMX event hooks — game-style feedback
var actionMessages = {
    start:   { pending: '.. STARTING...',      ok: '>> CONTAINER STARTED!',    fail: '!! START FAILED' },
    stop:    { pending: '.. STOPPING...',       ok: '>> CONTAINER STOPPED.',     fail: '!! STOP FAILED' },
    restart: { pending: '.. RESTARTING...',     ok: '>> CONTAINER RESTARTED!',   fail: '!! RESTART FAILED' },
    update:  { pending: '.. UPDATING...',       ok: '>> UPDATE COMPLETE!',       fail: '!! UPDATE FAILED' }
};

function matchAction(el) {
    if (!el || !el.getAttribute) return null;
    var url = el.getAttribute('hx-post') || el.getAttribute('hx-delete') || '';
    var match = url.match(/\/api\/services\/[^/]+\/(start|stop|restart|update)/);
    return match ? match[1] : null;
}

// Immediate feedback when button is clicked
document.addEventListener('htmx:beforeRequest', function(e) {
    var action = matchAction(e.detail.elt);
    if (!action || !actionMessages[action]) return;
    showToast(actionMessages[action].pending, 'info');
});

// Success feedback when operation completes
document.addEventListener('htmx:afterRequest', function(e) {
    var action = matchAction(e.detail.elt);
    if (!action || !actionMessages[action]) return;

    if (e.detail.successful) {
        showToast(actionMessages[action].ok, 'success');
    }
});

// Error feedback
document.addEventListener('htmx:responseError', function(e) {
    var action = matchAction(e.detail.elt);
    var status = e.detail.xhr.status;

    if (status === 409) {
        showToast('!! BUSY — operation in progress', 'warning');
    } else if (action && actionMessages[action]) {
        showToast(actionMessages[action].fail, 'error');
    } else {
        showToast('!! ERROR: ' + status, 'error');
    }
});

// --- Overview: stagger-fade only on initial load/navigation, not on polls ---

function applyStaggerToGrids() {
    document.querySelectorAll('.grid-cards').forEach(function(el) {
        el.classList.add('stagger-fade');
        setTimeout(function() { el.classList.remove('stagger-fade'); }, 800);
    });
}

document.addEventListener('DOMContentLoaded', applyStaggerToGrids);

// --- Seeders ---

function setSeederStatus(name, status) {
    sessionStorage.setItem('seederStatus-' + name, status);
    var card = document.getElementById('seeder-card-' + name);
    var badge = document.getElementById('seeder-status-' + name);
    if (card) {
        card.className = card.className.replace(/seeder-(idle|running|success|error)/g, '').trim();
        card.classList.add('seeder-' + status);
    }
    if (badge) {
        badge.className = 'badge seeder-status';
        var labels = { idle: 'IDLE', running: 'RUNNING', success: 'OK', error: 'FAIL' };
        var classes = { idle: 'badge-ghost', running: 'badge-warning', success: 'badge-success', error: 'badge-error' };
        badge.classList.add(classes[status] || 'badge-ghost');
        badge.textContent = labels[status] || status.toUpperCase();
    }
}

function showSeederLog(name) {
    var panel = document.getElementById('seeder-log-' + name);
    var toggle = document.getElementById('seeder-toggle-' + name);
    if (!panel) return;
    panel.classList.remove('hidden');
    if (toggle) {
        toggle.style.display = '';
        var chevron = toggle.querySelector('.seeder-chevron');
        if (chevron) chevron.style.transform = 'rotate(-270deg)';
    }
}

function toggleSeederLog(name) {
    var panel = document.getElementById('seeder-log-' + name);
    var toggle = document.getElementById('seeder-toggle-' + name);
    if (!panel) return;
    var isHidden = panel.classList.toggle('hidden');
    if (toggle) {
        var chevron = toggle.querySelector('.seeder-chevron');
        if (chevron) chevron.style.transform = isHidden ? 'rotate(-90deg)' : 'rotate(-270deg)';
    }
}

function appendSeederLog(name, text) {
    var output = document.getElementById('seeder-log-output-' + name);
    var line = '<div>' + text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;') + '</div>';
    var stored = sessionStorage.getItem('seederLog-' + name) || '';
    stored += line;
    sessionStorage.setItem('seederLog-' + name, stored);
    if (output) {
        output.innerHTML = stored;
        output.scrollTop = output.scrollHeight;
    }
}

// Restore seeder statuses and per-card logs when seeders partial is loaded
document.addEventListener('htmx:afterSwap', function(e) {
    if (e.detail.target && e.detail.target.id !== 'main-content') return;

    // Apply stagger animation on navigation
    applyStaggerToGrids();

    if (!document.getElementById('seeders-page')) return;

    // Restore statuses and logs from sessionStorage
    document.querySelectorAll('.seeder-item').forEach(function(card) {
        var name = card.id.replace('seeder-card-', '');

        // Prefer live status from sessionStorage over server-rendered data-last-status
        var liveStatus = sessionStorage.getItem('seederStatus-' + name);
        var serverStatus = card.dataset.lastStatus;
        var status = liveStatus || serverStatus;
        if (status) setSeederStatus(name, status);

        var logHtml = sessionStorage.getItem('seederLog-' + name);
        if (logHtml) {
            var output = document.getElementById('seeder-log-output-' + name);
            if (output) {
                output.innerHTML = logHtml;
                output.scrollTop = output.scrollHeight;
                showSeederLog(name);
            }
        }
    });
});

function seederSSE(url, btn, name) {
    var originalText = btn.querySelector('span') ? btn.querySelector('span').textContent : '';
    if (btn.querySelector('span')) btn.querySelector('span').textContent = 'Running...';
    btn.disabled = true;

    if (name) {
        setSeederStatus(name, 'running');
        var out = document.getElementById('seeder-log-output-' + name);
        if (out) { out.innerHTML = ''; sessionStorage.removeItem('seederLog-' + name); }
        showSeederLog(name);
    } else {
        document.querySelectorAll('.seeder-card').forEach(function(c) {
            var n = c.id.replace('seeder-card-', '');
            setSeederStatus(n, 'running');
            var out = document.getElementById('seeder-log-output-' + n);
            if (out) { out.innerHTML = ''; sessionStorage.removeItem('seederLog-' + n); }
            showSeederLog(n);
        });
    }

    var source = new EventSource(url);

    source.onmessage = function(e) {
        if (name) {
            appendSeederLog(name, e.data);
        } else {
            var match = e.data.match(/^\[([^\]]+)\]/);
            if (match) appendSeederLog(match[1], e.data);
        }
    };

    source.addEventListener('done', function(e) {
        source.close();
        btn.disabled = false;
        if (btn.querySelector('span')) btn.querySelector('span').textContent = originalText;
        if (name) {
            setSeederStatus(name, 'success');
            appendSeederLog(name, '✓ ' + (e.data || 'completed'));
            showToast('Seeder completed', 'success');
        } else {
            document.querySelectorAll('.seeder-card').forEach(function(c) {
                setSeederStatus(c.id.replace('seeder-card-', ''), 'success');
            });
            showToast('Seeders completed', 'success');
        }
    });

    source.addEventListener('error', function(e) {
        source.close();
        btn.disabled = false;
        if (btn.querySelector('span')) btn.querySelector('span').textContent = originalText;
        showToast('Seeder failed', 'error');
        if (name) {
            setSeederStatus(name, 'error');
            appendSeederLog(name, '✗ ' + (e.data || 'failed'));
        } else {
            var errMatch = e.data ? e.data.match(/seeder "([^"]+)"/) : null;
            var failedName = errMatch ? errMatch[1] : null;
            document.querySelectorAll('.seeder-card').forEach(function(c) {
                var n = c.id.replace('seeder-card-', '');
                if (n === failedName) {
                    setSeederStatus(n, 'error');
                    appendSeederLog(n, '✗ ' + (e.data || 'failed'));
                } else if (c.classList.contains('seeder-running')) {
                    setSeederStatus(n, 'idle');
                }
            });
        }
    });
}

function runAllSeeders(btn) {
    seederSSE('/api/seeders/run', btn, null);
}

function runOneSeeder(btn, name) {
    seederSSE('/api/seeders/run/' + encodeURIComponent(name), btn, name);
}
