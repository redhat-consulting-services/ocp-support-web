(function() {
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    const jobsContainer = document.getElementById('jobs-container');
    const jobsEmpty = document.getElementById('jobs-empty');
    let pollInterval = null;
    let activeJobs = {};

    // Anonymize toggle
    let anonymizeEnabled = false;
    const anonHint = document.getElementById('anon-hint');
    document.querySelectorAll('#anonymize-toggle [data-anon]').forEach(btn => {
        btn.addEventListener('click', () => {
            anonymizeEnabled = btn.dataset.anon === 'true';
            document.querySelectorAll('#anonymize-toggle .pf-v5-c-toggle-group__button').forEach(b => b.classList.remove('pf-m-selected'));
            btn.classList.add('pf-m-selected');
            anonHint.textContent = anonymizeEnabled ? 'IPs, MACs and domain names will be obfuscated' : 'Data will be sent as-is';
        });
    });
    document.querySelector('#anonymize-toggle [data-anon="false"]').classList.add('pf-m-selected');

    // Since (time frame) toggle
    let sinceEnabled = false;
    const sinceSelect = document.getElementById('since-select');
    const sinceHint = document.getElementById('since-hint');
    document.querySelectorAll('#since-toggle [data-since-enabled]').forEach(btn => {
        btn.addEventListener('click', () => {
            sinceEnabled = btn.dataset.sinceEnabled === 'true';
            document.querySelectorAll('#since-toggle .pf-v5-c-toggle-group__button').forEach(b => b.classList.remove('pf-m-selected'));
            btn.classList.add('pf-m-selected');
            sinceSelect.disabled = !sinceEnabled;
            updateSinceHint();
        });
    });
    sinceSelect.addEventListener('change', updateSinceHint);
    function updateSinceHint() {
        if (!sinceEnabled) {
            sinceHint.textContent = 'Collect all logs (no time limit)';
        } else {
            const val = sinceSelect.value;
            sinceHint.textContent = 'Only collect logs from the last ' + val.replace('h', ' hours');
        }
    }

    // Gather type card selection
    let selectedType = 'default';
    document.querySelectorAll('.gather-type-card').forEach(card => {
        card.addEventListener('click', () => {
            document.querySelectorAll('.gather-type-card').forEach(c => c.classList.remove('selected'));
            card.classList.add('selected');
            selectedType = card.dataset.type;
        });
    });

    const startBtn = document.getElementById('start-gather-btn');
    startBtn.addEventListener('click', () => startGather());

    async function startGather() {
        const type = selectedType;
        startBtn.disabled = true;
        startBtn.textContent = 'Starting...';
        const anonymize = anonymizeEnabled;
        const since = sinceEnabled ? sinceSelect.value : '';
        try {
            const res = await fetch('/api/support/gather', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({type, anonymize, since})
            });
            const data = await res.json();
            if (data.error) {
                alert('Error: ' + data.error);
                return;
            }
            activeJobs[data.id] = true;
            addJobCard(data.id, type, anonymize, since);
            startPolling();
        } catch (e) {
            alert('Failed to start gather: ' + e.message);
        } finally {
            startBtn.disabled = false;
            startBtn.textContent = 'Start Gathering';
        }
    }

    function labelFor(type) {
        const labels = {
            'default': 'Default Must-Gather',
            'virtualization': 'Virtualization',
            'odf': 'ODF (Storage)',
            'acm': 'ACM',
            'logging': 'Logging',
            'service-mesh': 'Service Mesh',
            'compliance': 'Compliance',
            'mtc': 'MTC',
            'gitops': 'GitOps',
            'serverless': 'Serverless',
            'audit': 'Audit Logs',
            'all': 'Gather All',
            'etcd-backup': 'Etcd Backup'
        };
        return labels[type] || type;
    }

    function formatElapsed(startedAt) {
        const start = new Date(startedAt);
        const now = new Date();
        const secs = Math.floor((now - start) / 1000);
        const m = Math.floor(secs / 60);
        const s = secs % 60;
        if (m > 0) return m + 'm ' + s + 's';
        return s + 's';
    }

    function addJobCard(id, type, anonymize, since) {
        jobsEmpty.classList.add('hidden');
        if (document.getElementById('job-' + id)) return;

        const safeId = escapeHtml(id);
        const safeLabel = escapeHtml(labelFor(type));
        const sinceLabel = since ? ' (' + escapeHtml(since.replace('h', ' hours')) + ')' : '';
        const card = document.createElement('div');
        card.id = 'job-' + id;
        card.className = 'pf-v5-c-card pf-v5-u-mb-md';
        card.innerHTML = `
            <div class="pf-v5-c-card__title">
                <div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center">
                    <div class="pf-v5-l-flex pf-m-gap-sm pf-m-align-items-center">
                        <h3 class="pf-v5-c-card__title-text">${safeLabel}${anonymize ? ' (anonymized)' : ''}${sinceLabel}</h3>
                        <span class="pf-v5-c-label pf-m-blue" id="status-${safeId}">
                            <span class="pf-v5-c-label__content">Running</span>
                        </span>
                        <span class="pf-v5-u-font-size-sm pf-v5-u-color-200" id="elapsed-${safeId}"></span>
                    </div>
                    <div id="actions-${safeId}" class="hidden"></div>
                </div>
            </div>
            <div class="pf-v5-c-card__body">
                <div class="pf-v5-u-mb-sm pf-v5-u-font-size-sm" id="step-label-${safeId}">Initializing...</div>
                <div class="pf-v5-c-progress pf-v5-u-mb-md" id="progress-${safeId}">
                    <div class="pf-v5-c-progress__description" id="progress-text-${safeId}"></div>
                    <div class="pf-v5-c-progress__bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0">
                        <div class="pf-v5-c-progress__indicator" id="progress-bar-${safeId}" style="width: 0%; transition: width 0.5s ease;">
                            <span class="pf-v5-c-progress__measure" id="progress-pct-${safeId}"></span>
                        </div>
                    </div>
                </div>
                <pre class="pf-v5-u-font-size-xs" id="log-${safeId}" style="max-height:300px;overflow-y:auto;background:#1b1d21;color:#d2d2d2;padding:10px;border-radius:4px;white-space:pre-wrap;font-family:'Red Hat Mono',monospace;line-height:1.4;"></pre>
            </div>`;
        jobsContainer.prepend(card);
    }

    function startPolling() {
        if (pollInterval) return;
        pollInterval = setInterval(pollJobs, 2000);
        pollJobs();
    }

    async function pollJobs() {
        const ids = Object.keys(activeJobs);
        if (ids.length === 0) {
            clearInterval(pollInterval);
            pollInterval = null;
            return;
        }

        for (const id of ids) {
            try {
                const res = await fetch('/api/support/gather/' + encodeURIComponent(id));
                const job = await res.json();
                if (job.error && !job.status) continue;
                updateJobUI(job);
                if (job.status === 'complete' || job.status === 'failed') {
                    delete activeJobs[id];
                }
            } catch (e) {
                // ignore transient errors
            }
        }
    }

    function updateJobUI(job) {
        const logEl = document.getElementById('log-' + job.id);
        const statusEl = document.getElementById('status-' + job.id);
        const progressEl = document.getElementById('progress-' + job.id);
        const progressBar = document.getElementById('progress-bar-' + job.id);
        const progressPct = document.getElementById('progress-pct-' + job.id);
        const progressText = document.getElementById('progress-text-' + job.id);
        const stepLabel = document.getElementById('step-label-' + job.id);
        const elapsedEl = document.getElementById('elapsed-' + job.id);
        const actionsEl = document.getElementById('actions-' + job.id);
        if (!logEl) return;

        if (job.logOutput) {
            logEl.textContent = job.logOutput;
            logEl.scrollTop = logEl.scrollHeight;
        }

        if (elapsedEl && job.startedAt) {
            elapsedEl.textContent = formatElapsed(job.startedAt);
        }

        if (job.totalSteps > 0 && progressBar) {
            const pct = Math.round((job.step / job.totalSteps) * 100);
            progressBar.style.width = pct + '%';
            progressBar.setAttribute('aria-valuenow', pct);
            progressPct.textContent = pct + '%';
            progressText.textContent = 'Step ' + job.step + ' of ' + job.totalSteps;
        }

        if (stepLabel && job.stepLabel) {
            stepLabel.textContent = job.stepLabel;
        }

        if (job.status === 'complete') {
            if (job.warning) {
                statusEl.className = 'pf-v5-c-label pf-m-orange';
                statusEl.innerHTML = '<span class="pf-v5-c-label__content">Completed with errors</span>';
                if (stepLabel) stepLabel.textContent = job.warning;
            } else {
                statusEl.className = 'pf-v5-c-label pf-m-green';
                statusEl.innerHTML = '<span class="pf-v5-c-label__content">Complete</span>';
                if (stepLabel) stepLabel.textContent = 'Archive ready for download';
            }
            if (progressBar) {
                progressBar.style.width = '100%';
                progressPct.textContent = '100%';
            }
            progressText.textContent = 'Done';
            actionsEl.classList.remove('hidden');
            actionsEl.innerHTML = `<a href="/api/support/gather/${encodeURIComponent(job.id)}/download" class="pf-v5-c-button pf-m-primary" style="display:inline-flex;align-items:center;gap:6px;">
                <svg style="width:16px;height:16px;fill:currentColor" viewBox="0 0 16 16"><path d="M8 1a.5.5 0 0 1 .5.5v8.793l2.146-2.147a.5.5 0 0 1 .708.708l-3 3a.5.5 0 0 1-.708 0l-3-3a.5.5 0 1 1 .708-.708L7.5 10.293V1.5A.5.5 0 0 1 8 1z"/><path d="M2 13.5a.5.5 0 0 1 .5-.5h11a.5.5 0 0 1 0 1h-11a.5.5 0 0 1-.5-.5z"/></svg>
                Download ${escapeHtml(labelFor(job.type))}</a>`;
        } else if (job.status === 'failed') {
            statusEl.className = 'pf-v5-c-label pf-m-red';
            statusEl.innerHTML = '<span class="pf-v5-c-label__content">Failed</span>';
            progressEl.classList.add('hidden');
            if (stepLabel) {
                if (job.error && job.error.includes('timed out')) {
                    stepLabel.textContent = 'Must-gather timed out after 30 minutes';
                } else {
                    stepLabel.textContent = job.error || 'Failed';
                }
            }
        }
    }

    async function loadExistingJobs() {
        try {
            const res = await fetch('/api/support/jobs');
            const jobs = await res.json();
            if (!jobs || jobs.length === 0) return;
            jobsEmpty.classList.add('hidden');
            jobs.sort((a, b) => new Date(b.startedAt) - new Date(a.startedAt));
            for (const job of jobs) {
                addJobCard(job.id, job.type, job.anonymize, job.since);
                updateJobUI(job);
                if (job.status === 'running') {
                    activeJobs[job.id] = true;
                }
            }
            if (Object.keys(activeJobs).length > 0) {
                startPolling();
            }
        } catch (e) {
            // ignore
        }
    }

    // Etcd backup button
    const etcdBtn = document.getElementById('start-etcd-backup-btn');
    if (etcdBtn) {
        etcdBtn.addEventListener('click', async () => {
            if (!confirm('Start an etcd backup? This will connect to a master node and create a snapshot.')) return;
            etcdBtn.disabled = true;
            etcdBtn.textContent = 'Starting...';
            try {
                const res = await fetch('/api/support/gather', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({type: 'etcd-backup', anonymize: false})
                });
                const data = await res.json();
                if (data.error) {
                    alert('Error: ' + data.error);
                    return;
                }
                activeJobs[data.id] = true;
                addJobCard(data.id, 'etcd-backup', false, '');
                startPolling();
            } catch (e) {
                alert('Failed to start etcd backup: ' + e.message);
            } finally {
                etcdBtn.disabled = false;
                etcdBtn.textContent = 'Start Etcd Backup';
            }
        });
    }

    // Cluster UUID
    async function loadClusterID() {
        try {
            const res = await fetch('/api/support/cluster-id');
            const data = await res.json();
            if (data.clusterID) {
                document.getElementById('cluster-uuid').textContent = data.clusterID;
            } else {
                document.getElementById('cluster-uuid').textContent = 'Unavailable';
            }
        } catch (e) {
            document.getElementById('cluster-uuid').textContent = 'Error loading';
        }
    }

    const copyBtn = document.getElementById('copy-uuid-btn');
    if (copyBtn) {
        copyBtn.addEventListener('click', () => {
            const uuid = document.getElementById('cluster-uuid').textContent;
            if (!uuid || uuid === 'Loading...' || uuid === 'Unavailable') return;
            navigator.clipboard.writeText(uuid).then(() => {
                const msg = document.getElementById('copy-uuid-msg');
                msg.classList.remove('hidden');
                setTimeout(() => msg.classList.add('hidden'), 2000);
            });
        });
    }

    // --- Etcd Diagnostics ---
    const diagResults = document.getElementById('diag-results');
    const diagObjectSelect = document.getElementById('diag-object-type');
    let diagCounter = 0;

    document.querySelectorAll('.diag-btn').forEach(btn => {
        // Store the original label so we can always restore it
        btn.dataset.label = btn.textContent.trim();
        btn.addEventListener('click', () => runDiag(btn.dataset.diag, btn));
    });

    async function runDiag(type, btn) {
        if (btn.disabled) return;

        const needsObject = (type === 'creation-timeline' || type === 'ns-object-counts');
        let objectType = '';
        if (needsObject) {
            objectType = diagObjectSelect.value;
            if (!objectType) {
                diagObjectSelect.focus();
                diagObjectSelect.style.borderColor = '#c9190b';
                setTimeout(() => diagObjectSelect.style.borderColor = '', 2000);
                return;
            }
        }

        const label = btn.dataset.label;
        btn.disabled = true;
        btn.innerHTML = '<span class="btn-spinner"></span>Running...';

        try {
            const res = await fetch('/api/support/etcd-diag', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({type, objectType})
            });
            const data = await res.json();
            if (data.error) {
                showDiagResult(type, objectType, null, data.error);
                btn.disabled = false;
                btn.textContent = label;
                return;
            }
            pollDiag(data.id, type, objectType, btn, label);
        } catch (e) {
            showDiagResult(type, objectType, null, e.message);
            btn.disabled = false;
            btn.textContent = label;
        }
    }

    async function pollDiag(id, type, objectType, btn, label) {
        const restoreBtn = () => {
            btn.disabled = false;
            btn.textContent = label;
        };

        // Unique placeholder ID per invocation
        const placeholderId = 'diag-running-' + (++diagCounter);
        const title = diagTitle(type, objectType);
        const placeholder = document.createElement('div');
        placeholder.id = placeholderId;
        placeholder.className = 'pf-v5-c-card pf-v5-u-mb-md';
        placeholder.style.border = '1px solid #d2d2d2';
        placeholder.innerHTML = `<div class="pf-v5-c-card__title"><h3 class="pf-v5-c-card__title-text">${escapeHtml(title)}</h3></div>
            <div class="pf-v5-c-card__body pf-v5-u-text-align-center pf-v5-u-py-lg">
                <span class="pf-v5-c-spinner pf-m-md" role="progressbar"><span class="pf-v5-c-spinner__clipper"></span><span class="pf-v5-c-spinner__lead-ball"></span><span class="pf-v5-c-spinner__tail-ball"></span></span>
                <p class="pf-v5-u-mt-sm pf-v5-u-font-size-sm">Running diagnostic... This may take a few minutes on large clusters.</p>
            </div>`;
        diagResults.prepend(placeholder);

        let notFoundRetries = 0;
        const poll = async () => {
            try {
                const res = await fetch('/api/support/etcd-diag/' + encodeURIComponent(id));
                if (res.status === 404 && notFoundRetries < 3) {
                    notFoundRetries++;
                    setTimeout(poll, 3000);
                    return;
                }
                const dj = await res.json();
                if (dj.status === 'running') {
                    setTimeout(poll, 2000);
                    return;
                }
                restoreBtn();
                const ph = document.getElementById(placeholderId);
                if (ph) ph.remove();
                if (dj.status === 'complete') {
                    showDiagResult(type, objectType, dj.output, null);
                } else {
                    showDiagResult(type, objectType, dj.output, dj.error || 'job not found');
                }
            } catch (e) {
                restoreBtn();
                const ph = document.getElementById(placeholderId);
                if (ph) ph.remove();
                showDiagResult(type, objectType, null, e.message);
            }
        };
        poll();
    }

    function diagTitle(type, objectType) {
        const titles = {
            'object-sizes': 'Object Sizes in Etcd',
            'object-counts': 'Object Type Counts',
            'ns-breakdown': 'Namespace Size Breakdown (secrets/configmaps/events)',
            'creation-timeline': 'Creation Timeline: ' + objectType,
            'ns-object-counts': 'Count per Namespace: ' + objectType
        };
        return titles[type] || type;
    }

    function showDiagResult(type, objectType, output, error) {
        // Remove running placeholder
        const placeholder = document.getElementById('diag-running-' + type);
        if (placeholder) placeholder.remove();

        const title = diagTitle(type, objectType);
        const resultId = 'diag-result-' + Date.now();
        const card = document.createElement('div');
        card.className = 'pf-v5-c-card pf-v5-u-mb-md';

        let bodyHtml = '';
        if (error) {
            bodyHtml = `<div class="pf-v5-u-danger-color-100 pf-v5-u-mb-md">Error: ${escapeHtmlDiag(error)}</div>`;
        }
        if (output) {
            bodyHtml += `<pre id="${resultId}" style="max-height:400px;overflow:auto;background:#1b1d21;color:#d2d2d2;padding:10px;border-radius:4px;white-space:pre-wrap;font-family:'Red Hat Mono',monospace;font-size:12px;line-height:1.4;">${escapeHtmlDiag(output)}</pre>`;
        } else if (!error) {
            bodyHtml = '<span class="pf-v5-u-color-200">No output returned</span>';
        }

        card.innerHTML = `<div class="pf-v5-c-card__title">
                <div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center">
                    <h3 class="pf-v5-c-card__title-text">${escapeHtml(title)}</h3>
                    <div class="pf-v5-l-flex pf-m-gap-sm">
                        ${output ? `<button class="pf-v5-c-button pf-m-secondary pf-m-small" onclick="copyDiagOutput('${escapeHtml(resultId)}')">Copy</button>
                        <button class="pf-v5-c-button pf-m-secondary pf-m-small" onclick="saveDiagOutput('${escapeHtml(resultId)}', '${escapeHtml(type)}')">Save .txt</button>` : ''}
                        <button class="pf-v5-c-button pf-m-plain pf-m-small" onclick="this.closest('.pf-v5-c-card').remove()" title="Dismiss">&times;</button>
                    </div>
                </div>
            </div>
            <div class="pf-v5-c-card__body">${bodyHtml}</div>`;

        diagResults.prepend(card);
    }

    function escapeHtmlDiag(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    window.copyDiagOutput = function(id) {
        const el = document.getElementById(id);
        if (!el) return;
        navigator.clipboard.writeText(el.textContent).then(() => {
            const btn = el.closest('.pf-v5-c-card').querySelector('[onclick*="copyDiag"]');
            if (btn) {
                const orig = btn.textContent;
                btn.textContent = 'Copied!';
                setTimeout(() => btn.textContent = orig, 2000);
            }
        });
    };

    window.saveDiagOutput = function(id, type) {
        const el = document.getElementById(id);
        if (!el) return;
        const blob = new Blob([el.textContent], {type: 'text/plain'});
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'etcd-diag-' + type + '-' + new Date().toISOString().slice(0,19).replace(/[T:]/g, '-') + '.txt';
        a.click();
        URL.revokeObjectURL(url);
    };

    async function loadCapabilities() {
        try {
            const res = await fetch('/api/support/capabilities');
            const caps = await res.json();
            const cnvCard = document.querySelector('.gather-type-card[data-type="virtualization"]');
            const odfCard = document.querySelector('.gather-type-card[data-type="odf"]');
            const acmCard = document.querySelector('.gather-type-card[data-type="acm"]');
            if (caps.cnv && cnvCard) cnvCard.style.display = '';
            if (caps.odf && odfCard) odfCard.style.display = '';
            if (caps.acm && acmCard) {
                acmCard.style.display = '';
                if (caps.acmVersion) {
                    acmCard.querySelector('.gather-type-card__desc').textContent =
                        'Advanced Cluster Management v' + caps.acmVersion;
                }
            }
            const capMap = {
                logging: {type: 'logging'},
                serviceMesh: {type: 'service-mesh', versionKey: 'serviceMeshVersion', label: 'Service Mesh'},
                compliance: {type: 'compliance'},
                mtc: {type: 'mtc', versionKey: 'mtcVersion', label: 'MTC'},
                gitops: {type: 'gitops', versionKey: 'gitopsVersion', label: 'GitOps'},
                serverless: {type: 'serverless', versionKey: 'serverlessVersion', label: 'Serverless'}
            };
            for (const [capKey, info] of Object.entries(capMap)) {
                if (caps[capKey]) {
                    const card = document.querySelector('.gather-type-card[data-type="' + info.type + '"]');
                    if (card) {
                        card.style.display = '';
                        if (info.versionKey && caps[info.versionKey]) {
                            card.querySelector('.gather-type-card__desc').textContent =
                                card.querySelector('.gather-type-card__desc').textContent + ' v' + caps[info.versionKey];
                        }
                    }
                }
            }
        } catch (e) {
            // If capabilities check fails, show all cards
        }
    }

    loadCapabilities();
    loadClusterID();
    loadExistingJobs();
})();
