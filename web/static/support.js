(function() {
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    const jobsContainer = document.getElementById('jobs-container');
    const jobsEmpty = document.getElementById('jobs-empty');
    const toastContainer = document.getElementById('toast-container');
    let pollInterval = null;
    let activeJobs = {};

    function showToast(title, message, variant, jobId) {
        variant = variant || 'info';
        const iconMap = {
            success: '<svg class="pf-v5-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" width="1em" height="1em"><path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z"/></svg>',
            info: '<svg class="pf-v5-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" width="1em" height="1em"><path d="M256 8C119.043 8 8 119.083 8 256c0 136.997 111.043 248 248 248s248-111.003 248-248C504 119.083 392.957 8 256 8zm0 110c23.196 0 42 18.804 42 42s-18.804 42-42 42-42-18.804-42-42 18.804-42 42-42zm56 254c0 6.627-5.373 12-12 12h-88c-6.627 0-12-5.373-12-12v-24c0-6.627 5.373-12 12-12h12v-64h-12c-6.627 0-12-5.373-12-12v-24c0-6.627 5.373-12 12-12h64c6.627 0 12 5.373 12 12v100h12c6.627 0 12 5.373 12 12v24z"/></svg>',
            danger: '<svg class="pf-v5-svg" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" width="1em" height="1em"><path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"/></svg>'
        };
        let descHtml = '';
        if (message) descHtml += '<p>' + escapeHtml(message) + '</p>';
        if (jobId) descHtml += '<p style="margin-top:4px"><a href="#job-' + escapeHtml(jobId) + '" style="color:var(--pf-v5-global--link--Color);text-decoration:underline;cursor:pointer;" onclick="var el=document.getElementById(\'job-' + escapeHtml(jobId) + '\');if(el){el.scrollIntoView({behavior:\'smooth\',block:\'center\'});this.closest(\'.pf-v5-c-alert-group__item\').remove();}return false;">View progress ↓</a></p>';
        const li = document.createElement('li');
        li.className = 'pf-v5-c-alert-group__item';
        li.innerHTML = `
            <div class="pf-v5-c-alert pf-m-${variant}" aria-label="${escapeHtml(title)}">
                <div class="pf-v5-c-alert__icon">${iconMap[variant] || iconMap.info}</div>
                <p class="pf-v5-c-alert__title"><span class="pf-screen-reader">${escapeHtml(variant)}:</span>${escapeHtml(title)}</p>
                ${descHtml ? '<div class="pf-v5-c-alert__description">' + descHtml + '</div>' : ''}
                <div class="pf-v5-c-alert__action">
                    <button class="pf-v5-c-button pf-m-plain" type="button" aria-label="Close" onclick="this.closest('.pf-v5-c-alert-group__item').remove()">
                        <span class="pf-v5-c-button__icon"><svg viewBox="0 0 352 512" fill="currentColor" width="1em" height="1em"><path d="M242.72 256l100.07-100.07c12.28-12.28 12.28-32.19 0-44.48l-22.24-22.24c-12.28-12.28-32.19-12.28-44.48 0L176 189.28 75.93 89.21c-12.28-12.28-32.19-12.28-44.48 0L9.21 111.45c-12.28 12.28-12.28 32.19 0 44.48L109.28 256 9.21 356.07c-12.28 12.28-12.28 32.19 0 44.48l22.24 22.24c12.28 12.28 32.2 12.28 44.48 0L176 322.72l100.07 100.07c12.28 12.28 32.2 12.28 44.48 0l22.24-22.24c12.28-12.28 12.28-32.19 0-44.48L242.72 256z"/></svg></span>
                    </button>
                </div>
            </div>`;
        toastContainer.appendChild(li);
        setTimeout(() => { if (li.parentNode) li.remove(); }, 8000);
    }

    window.stopJob = async function(jobId, type) {
        if (!confirm('Are you sure you want to stop the ' + labelFor(type) + ' job?')) return;
        const btn = document.getElementById('stop-' + jobId);
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Stopping...';
        }
        try {
            await fetch('/api/support/gather/' + encodeURIComponent(jobId) + '/stop', { method: 'POST' });
        } catch (e) {
            console.error('Failed to stop job', e);
        }
    };

    // Anonymize Off/On toggle + granular checkboxes
    const anonHint = document.getElementById('anon-hint');
    const anonOptions = document.getElementById('anon-options');
    const anonIPs = document.getElementById('anon-ips');
    const anonMACs = document.getElementById('anon-macs');
    const anonDomains = document.getElementById('anon-domains');
    const anonServices = document.getElementById('anon-services');
    const anonCheckboxes = [anonIPs, anonMACs, anonDomains, anonServices];
    let anonymizeEnabled = false;

    function updateAnonHint() {
        if (!anonymizeEnabled) {
            anonHint.textContent = 'Data will be sent as-is';
            return;
        }
        const selected = [];
        if (anonIPs.checked) selected.push('IPs');
        if (anonMACs.checked) selected.push('MACs');
        if (anonDomains.checked) selected.push('domains');
        if (anonServices.checked) selected.push('service DNS');
        anonHint.textContent = selected.length > 0
            ? selected.join(', ') + ' will be obfuscated'
            : 'No options selected';
    }

    document.querySelectorAll('#anonymize-toggle [data-anon]').forEach(btn => {
        btn.addEventListener('click', () => {
            anonymizeEnabled = btn.dataset.anon === 'true';
            document.querySelectorAll('#anonymize-toggle .pf-v5-c-toggle-group__button').forEach(b => b.classList.remove('pf-m-selected'));
            btn.classList.add('pf-m-selected');
            if (anonymizeEnabled) {
                anonOptions.style.display = 'flex';
                anonCheckboxes.forEach(cb => cb.checked = true);
            } else {
                anonOptions.style.display = 'none';
            }
            updateAnonHint();
        });
    });
    anonCheckboxes.forEach(cb => cb.addEventListener('change', updateAnonHint));

    // Advanced options toggle
    const advancedToggle = document.getElementById('advanced-toggle');
    const advancedOptions = document.getElementById('advanced-options');
    const advancedArrow = document.getElementById('advanced-arrow');
    if (advancedToggle) {
        advancedToggle.addEventListener('click', () => {
            const hidden = advancedOptions.classList.toggle('hidden');
            advancedArrow.innerHTML = hidden ? '&#9654;' : '&#9660;';
        });
    }

    // Advanced: node name / node selector / host network
    const nodeNameSelect = document.getElementById('node-name-select');
    const nodeSelectorSelect = document.getElementById('node-selector-select');
    let hostNetworkEnabled = false;

    // Mutual exclusion: selecting node name clears node selector and vice versa
    if (nodeNameSelect) {
        nodeNameSelect.addEventListener('change', () => {
            if (nodeNameSelect.value) {
                nodeSelectorSelect.value = '';
                nodeSelectorSelect.disabled = true;
            } else {
                nodeSelectorSelect.disabled = false;
            }
        });
    }
    if (nodeSelectorSelect) {
        nodeSelectorSelect.addEventListener('change', () => {
            if (nodeSelectorSelect.value) {
                nodeNameSelect.value = '';
                nodeNameSelect.disabled = true;
            } else {
                nodeNameSelect.disabled = false;
            }
        });
    }

    // Host network toggle
    document.querySelectorAll('#host-network-toggle [data-hostnet]').forEach(btn => {
        btn.addEventListener('click', () => {
            hostNetworkEnabled = btn.dataset.hostnet === 'true';
            document.querySelectorAll('#host-network-toggle .pf-v5-c-toggle-group__button').forEach(b => b.classList.remove('pf-m-selected'));
            btn.classList.add('pf-m-selected');
        });
    });

    // Load nodes for dropdowns
    async function loadNodes() {
        try {
            const res = await fetch('/api/support/nodes');
            const nodes = await res.json();
            if (!Array.isArray(nodes) || nodes.length === 0) return;

            // Populate node name dropdown
            for (const node of nodes) {
                const opt = document.createElement('option');
                opt.value = node.name;
                opt.textContent = node.name;
                nodeNameSelect.appendChild(opt);
            }

            // Collect unique label key=value pairs (skip internal k8s labels)
            const internalLabelPattern = /kubernetes\.io\/|k8s\.io\//;
            const labelSet = new Set();
            for (const node of nodes) {
                if (!node.labels) continue;
                for (const [k, v] of Object.entries(node.labels)) {
                    if (internalLabelPattern.test(k)) continue;
                    labelSet.add(k + '=' + v);
                }
            }
            const sortedLabels = Array.from(labelSet).sort();
            for (const label of sortedLabels) {
                const opt = document.createElement('option');
                opt.value = label;
                opt.textContent = label;
                nodeSelectorSelect.appendChild(opt);
            }
        } catch (e) {
            // Nodes endpoint may not be available
        }
    }

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
        const anonOpts = {
            ips: anonIPs.checked,
            macs: anonMACs.checked,
            domains: anonDomains.checked,
            services: anonServices.checked
        };
        const anonymize = anonOpts.ips || anonOpts.macs || anonOpts.domains || anonOpts.services;
        const since = sinceEnabled ? sinceSelect.value : '';
        const nodeName = nodeNameSelect ? nodeNameSelect.value : '';
        const nodeSelector = nodeSelectorSelect ? nodeSelectorSelect.value : '';
        const hostNetwork = hostNetworkEnabled;
        try {
            const res = await fetch('/api/support/gather', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({type, anonymize, anonOpts, since, nodeName, nodeSelector, hostNetwork})
            });
            const data = await res.json();
            if (data.error) {
                showToast('Error starting gather', data.error, 'danger');
                return;
            }
            activeJobs[data.id] = true;
            addJobCard(data.id, type, anonymize, since);
            startPolling();
            showToast(labelFor(type) + ' started', 'Must-gather job is now running.', 'success', data.id);
        } catch (e) {
            showToast('Failed to start gather', e.message, 'danger');
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
            'mce': 'MCE',
            'netobserv': 'Network Observability',
            'local-storage': 'Local Storage',
            'sandboxed': 'Sandboxed Containers',
            'nhc': 'Node Health Check',
            'numa': 'NUMA Resources',
            'ptp': 'PTP',
            'secrets-store': 'Secrets Store CSI',
            'lvms': 'LVMS',
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
                    <div id="actions-${safeId}">
                        <button class="pf-v5-c-button pf-m-danger pf-m-small" id="stop-${safeId}" onclick="stopJob('${safeId}', '${escapeHtml(type)}')" style="display:inline-flex;align-items:center;gap:4px;">
                            <svg style="width:14px;height:14px;fill:currentColor" viewBox="0 0 16 16"><rect x="3" y="3" width="10" height="10" rx="1"/></svg>
                            Stop</button>
                    </div>
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
                <div class="step-tabs" id="step-tabs-${safeId}"></div>
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

    // Track which step tab is selected per job (null = auto-follow active step)
    const selectedStepTab = {};

    function parseSteps(logOutput) {
        const steps = [];
        if (!logOutput) return steps;
        const lines = logOutput.split('\n');
        let current = null;
        for (const line of lines) {
            const startMatch = line.match(/^=== Step (\d+)\/(\d+): (.+) ===$/);
            if (startMatch) {
                if (current) steps.push(current);
                current = { num: parseInt(startMatch[1]), total: parseInt(startMatch[2]), label: startMatch[3], lines: [], status: 'running' };
                continue;
            }
            const completeMatch = line.match(/^=== (.+) complete ===$/);
            if (completeMatch && current) {
                current.status = 'complete';
                current.lines.push(line);
                steps.push(current);
                current = null;
                continue;
            }
            const failedMatch = line.match(/^=== (.+) failed ===$/);
            if (failedMatch && current) {
                current.status = 'failed';
                current.lines.push(line);
                steps.push(current);
                current = null;
                continue;
            }
            if (current) {
                current.lines.push(line);
            }
        }
        if (current) steps.push(current);
        return steps;
    }

    function renderStepTabs(jobId, steps, jobStatus) {
        const tabsEl = document.getElementById('step-tabs-' + jobId);
        if (!tabsEl || steps.length <= 1) return;

        const activeStep = selectedStepTab[jobId];
        tabsEl.innerHTML = '';
        for (const step of steps) {
            const tab = document.createElement('span');
            tab.className = 'step-tab';
            if (step.status === 'running') tab.classList.add('step-running');
            else if (step.status === 'complete') tab.classList.add('step-complete');
            else if (step.status === 'failed') tab.classList.add('step-failed');

            const isActive = activeStep === step.num || (activeStep == null && step.status === 'running');
            if (isActive) tab.classList.add('active');

            let icon = '';
            if (step.status === 'complete') icon = '<span class="step-icon">✓</span>';
            else if (step.status === 'failed') icon = '<span class="step-icon">✗</span>';
            else if (step.status === 'running') icon = '<span class="step-icon btn-spinner" style="width:11px;height:11px;border-width:1.5px;margin:0;"></span>';

            tab.innerHTML = icon + escapeHtml(step.num + '. ' + step.label);
            tab.addEventListener('click', () => {
                selectedStepTab[jobId] = (selectedStepTab[jobId] === step.num) ? null : step.num;
                renderStepTabs(jobId, steps, jobStatus);
                showStepLog(jobId, steps);
            });
            tabsEl.appendChild(tab);
        }

        // When job completes and still auto-following, reset to show full log
        if ((jobStatus === 'complete' || jobStatus === 'failed') && activeStep == null) {
            selectedStepTab[jobId] = null;
        }
    }

    function showStepLog(jobId, steps) {
        const logEl = document.getElementById('log-' + jobId);
        if (!logEl) return;
        const active = selectedStepTab[jobId];
        if (active == null) {
            // Restore full log from data attribute
            if (logEl.dataset.fullLog) {
                logEl.textContent = logEl.dataset.fullLog;
                logEl.scrollTop = logEl.scrollHeight;
            }
            return;
        }
        const step = steps.find(s => s.num === active);
        if (step) {
            logEl.textContent = '=== Step ' + step.num + '/' + step.total + ': ' + step.label + ' ===\n' + step.lines.join('\n');
            logEl.scrollTop = logEl.scrollHeight;
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

        const steps = parseSteps(job.logOutput);
        renderStepTabs(job.id, steps, job.status);

        if (job.logOutput) {
            logEl.dataset.fullLog = job.logOutput;
            const active = selectedStepTab[job.id];
            if (active == null) {
                logEl.textContent = job.logOutput;
                logEl.scrollTop = logEl.scrollHeight;
            } else {
                showStepLog(job.id, steps);
            }
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
            const isStopped = job.error && job.error.includes('Stopped by user');
            statusEl.innerHTML = '<span class="pf-v5-c-label__content">' + (isStopped ? 'Stopped' : 'Failed') + '</span>';
            progressEl.classList.add('hidden');
            actionsEl.innerHTML = '';
            if (stepLabel) {
                if (isStopped) {
                    stepLabel.textContent = 'Stopped by user';
                } else if (job.error && job.error.includes('timed out')) {
                    stepLabel.textContent = 'Must-gather timed out after 60 minutes';
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
                    showToast('Error starting etcd backup', data.error, 'danger');
                    return;
                }
                activeJobs[data.id] = true;
                addJobCard(data.id, 'etcd-backup', false, '');
                startPolling();
                showToast('Etcd Backup started', 'Backup job is now running.', 'success', data.id);
            } catch (e) {
                showToast('Failed to start etcd backup', e.message, 'danger');
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
            if (caps.cnv && cnvCard) {
                cnvCard.style.display = '';
                if (caps.cnvVersion) {
                    cnvCard.querySelector('.gather-type-card__desc').textContent =
                        'OpenShift Virtualization v' + caps.cnvVersion;
                }
            }
            if (caps.odf && odfCard) {
                odfCard.style.display = '';
                if (caps.odfVersion) {
                    odfCard.querySelector('.gather-type-card__desc').textContent =
                        'OpenShift Data Foundation v' + caps.odfVersion;
                }
            }
            if (caps.acm && acmCard) {
                acmCard.style.display = '';
                if (caps.acmVersion) {
                    acmCard.querySelector('.gather-type-card__desc').textContent =
                        'Advanced Cluster Management v' + caps.acmVersion;
                }
            }
            const capMap = {
                logging: {type: 'logging', versionKey: 'loggingVersion', label: 'Logging'},
                serviceMesh: {type: 'service-mesh', versionKey: 'serviceMeshVersion', label: 'Service Mesh'},
                compliance: {type: 'compliance'},
                mtc: {type: 'mtc', versionKey: 'mtcVersion', label: 'MTC'},
                gitops: {type: 'gitops', versionKey: 'gitopsVersion', label: 'GitOps'},
                serverless: {type: 'serverless', versionKey: 'serverlessVersion', label: 'Serverless'},
                mce: {type: 'mce'},
                netObserv: {type: 'netobserv'},
                localStorage: {type: 'local-storage', versionKey: 'localStorageVersion', label: 'Local Storage'},
                sandboxed: {type: 'sandboxed', versionKey: 'sandboxedVersion', label: 'Sandboxed Containers'},
                nhc: {type: 'nhc', versionKey: 'nhcVersion', label: 'Node Health Check'},
                numa: {type: 'numa', versionKey: 'numaVersion', label: 'NUMA Resources'},
                ptp: {type: 'ptp', versionKey: 'ptpVersion', label: 'PTP'},
                secretsStore: {type: 'secrets-store', versionKey: 'secretsStoreVersion', label: 'Secrets Store CSI'},
                lvms: {type: 'lvms', versionKey: 'lvmsVersion', label: 'LVMS'}
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
            // Update "Gather All" description with detected operators
            const allDesc = document.getElementById('all-desc');
            if (allDesc) {
                const detected = ['Default'];
                if (caps.cnv) detected.push('Virtualization');
                if (caps.odf) detected.push('ODF');
                if (caps.acm) detected.push('ACM');
                if (caps.logging) detected.push('Logging');
                if (caps.serviceMesh) detected.push('Service Mesh');
                if (caps.compliance) detected.push('Compliance');
                if (caps.mtc) detected.push('MTC');
                if (caps.gitops) detected.push('GitOps');
                if (caps.serverless) detected.push('Serverless');
                if (caps.mce) detected.push('MCE');
                if (caps.netObserv) detected.push('NetObserv');
                if (caps.localStorage) detected.push('Local Storage');
                if (caps.sandboxed) detected.push('Sandboxed');
                if (caps.nhc) detected.push('NHC');
                if (caps.numa) detected.push('NUMA');
                if (caps.ptp) detected.push('PTP');
                if (caps.secretsStore) detected.push('Secrets Store');
                if (caps.lvms) detected.push('LVMS');
                detected.push('Audit');
                allDesc.textContent = detected.join(' + ');
            }
        } catch (e) {
            // If capabilities check fails, show all cards
        }
    }

    loadCapabilities();
    loadClusterID();
    loadNodes();
    loadExistingJobs();
})();
