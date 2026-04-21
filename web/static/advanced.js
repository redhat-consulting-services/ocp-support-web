(function() {
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    const jobsContainer = document.getElementById('jobs-container');
    const jobsEmpty = document.getElementById('jobs-empty');
    const toastContainer = document.getElementById('toast-container');
    const startBtn = document.getElementById('start-gather-btn');
    let pollInterval = null;
    let activeJobs = {};  // id → 'local' or 'remote'
    let allNamespaces = [];
    let selectedNamespaces = new Set();

    // --- Toast ---
    function showToast(title, message, variant, jobId) {
        variant = variant || 'info';
        const iconMap = {
            success: '<svg class="pf-v5-svg" viewBox="0 0 512 512" fill="currentColor" width="1em" height="1em"><path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z"/></svg>',
            danger: '<svg class="pf-v5-svg" viewBox="0 0 512 512" fill="currentColor" width="1em" height="1em"><path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"/></svg>'
        };
        let descHtml = '';
        if (message) descHtml += '<p>' + escapeHtml(message) + '</p>';
        if (jobId) descHtml += '<p style="margin-top:4px"><a href="#job-' + escapeHtml(jobId) + '" style="color:var(--pf-v5-global--link--Color);text-decoration:underline;cursor:pointer;" onclick="var el=document.getElementById(\'job-' + escapeHtml(jobId) + '\');if(el){el.scrollIntoView({behavior:\'smooth\',block:\'center\'});this.closest(\'.pf-v5-c-alert-group__item\').remove();}return false;">View progress</a></p>';
        const li = document.createElement('li');
        li.className = 'pf-v5-c-alert-group__item';
        li.innerHTML = '<div class="pf-v5-c-alert pf-m-' + variant + '"><div class="pf-v5-c-alert__icon">' + (iconMap[variant] || '') + '</div><p class="pf-v5-c-alert__title">' + escapeHtml(title) + '</p>' + (descHtml ? '<div class="pf-v5-c-alert__description">' + descHtml + '</div>' : '') + '<div class="pf-v5-c-alert__action"><button class="pf-v5-c-button pf-m-plain" type="button" onclick="this.closest(\'.pf-v5-c-alert-group__item\').remove()">&times;</button></div></div>';
        toastContainer.appendChild(li);
        setTimeout(function() { if (li.parentNode) li.remove(); }, 8000);
    }

    // --- Namespace loading ---
    async function loadNamespaces(cluster) {
        const loadingEl = document.getElementById('ns-loading');
        const errorEl = document.getElementById('ns-error');
        const containerEl = document.getElementById('ns-container');

        // Reset state
        allNamespaces = [];
        selectedNamespaces = new Set();
        document.getElementById('ns-rows').innerHTML = '';
        loadingEl.classList.remove('hidden');
        containerEl.classList.add('hidden');
        errorEl.classList.add('hidden');
        updateNsCount();

        try {
            var url = '/api/support/namespaces';
            if (cluster && cluster !== 'local') {
                url = '/api/acm/clusters/' + encodeURIComponent(cluster) + '/namespaces';
            }
            const res = await fetch(url);
            if (!res.ok) {
                const data = await res.json();
                throw new Error(data.error || 'Failed to load namespaces');
            }
            allNamespaces = await res.json();
            loadingEl.classList.add('hidden');
            containerEl.classList.remove('hidden');
            addNsRow();
        } catch (e) {
            loadingEl.classList.add('hidden');
            errorEl.textContent = 'Failed to load namespaces: ' + e.message;
            errorEl.classList.remove('hidden');
        }
    }

    function updateNsCount() {
        var countEl = document.getElementById('ns-count');
        var n = selectedNamespaces.size;
        countEl.textContent = n === 0 ? '' : n + ' namespace' + (n === 1 ? '' : 's') + ' selected';
        startBtn.disabled = n === 0;
    }

    function addNsRow(preselected) {
        var rowsEl = document.getElementById('ns-rows');
        var row = document.createElement('div');
        row.className = 'ns-row pf-v5-l-flex pf-m-align-items-center pf-m-gap-sm pf-v5-u-mb-sm';

        var select = document.createElement('select');
        select.className = 'pf-v5-c-form-control';
        select.style.maxWidth = '400px';
        select.style.flex = '1';

        var placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = '— Select a namespace —';
        select.appendChild(placeholder);

        for (var i = 0; i < allNamespaces.length; i++) {
            var ns = allNamespaces[i];
            if (selectedNamespaces.has(ns)) continue;
            var opt = document.createElement('option');
            opt.value = ns;
            opt.textContent = ns;
            select.appendChild(opt);
        }

        if (preselected) {
            // Re-add the preselected value as an option (it was filtered out above)
            var opt = document.createElement('option');
            opt.value = preselected;
            opt.textContent = preselected;
            select.appendChild(opt);
            select.value = preselected;
        }

        var removeBtn = document.createElement('button');
        removeBtn.className = 'pf-v5-c-button pf-m-plain';
        removeBtn.type = 'button';
        removeBtn.innerHTML = '<i class="fas fa-times" aria-hidden="true"></i>&times;';
        removeBtn.style.fontSize = '16px';
        removeBtn.style.lineHeight = '1';
        removeBtn.style.padding = '4px 8px';
        removeBtn.title = 'Remove';

        row.appendChild(select);
        row.appendChild(removeBtn);
        rowsEl.appendChild(row);

        select.addEventListener('change', function() {
            var prev = select.dataset.prev || '';
            var val = select.value;
            if (prev) selectedNamespaces.delete(prev);
            if (val) {
                selectedNamespaces.add(val);
                select.dataset.prev = val;
                // If this is the last row and a value was selected, add a new empty row
                if (row === rowsEl.lastElementChild) {
                    addNsRow();
                }
                // Update options in all other selects to remove this newly selected value
                refreshAllSelectOptions();
            } else {
                select.dataset.prev = '';
            }
            updateNsCount();
        });

        removeBtn.addEventListener('click', function() {
            var val = select.value;
            if (val) selectedNamespaces.delete(val);
            row.remove();
            // Ensure there's always at least one empty row
            var rows = rowsEl.querySelectorAll('.ns-row');
            var lastSelect = rows.length > 0 ? rows[rows.length - 1].querySelector('select') : null;
            if (rows.length === 0 || (lastSelect && lastSelect.value !== '')) {
                addNsRow();
            }
            refreshAllSelectOptions();
            updateNsCount();
        });

        return row;
    }

    function refreshAllSelectOptions() {
        var rowsEl = document.getElementById('ns-rows');
        var selects = rowsEl.querySelectorAll('select');
        for (var s = 0; s < selects.length; s++) {
            var sel = selects[s];
            var currentVal = sel.value;
            // Clear all but the placeholder
            while (sel.options.length > 1) sel.remove(1);
            // Re-populate with available namespaces
            for (var i = 0; i < allNamespaces.length; i++) {
                var ns = allNamespaces[i];
                if (ns === currentVal || !selectedNamespaces.has(ns)) {
                    var opt = document.createElement('option');
                    opt.value = ns;
                    opt.textContent = ns;
                    sel.appendChild(opt);
                }
            }
            sel.value = currentVal;
        }
    }

    // --- Resource type select/deselect ---
    document.getElementById('rt-select-all').addEventListener('click', function() {
        document.querySelectorAll('.rt-check').forEach(function(cb) { cb.checked = true; });
    });
    document.getElementById('rt-deselect-all').addEventListener('click', function() {
        document.querySelectorAll('.rt-check').forEach(function(cb) { cb.checked = false; });
    });

    // --- Pod logs toggle ---
    let includeLogsEnabled = true;
    document.querySelectorAll('#logs-toggle [data-logs]').forEach(function(btn) {
        btn.addEventListener('click', function() {
            includeLogsEnabled = btn.dataset.logs === 'true';
            document.querySelectorAll('#logs-toggle .pf-v5-c-toggle-group__button').forEach(function(b) { b.classList.remove('pf-m-selected'); });
            btn.classList.add('pf-m-selected');
        });
    });

    // --- Anonymize toggle ---
    let anonymizeEnabled = false;
    document.querySelectorAll('#anonymize-toggle [data-anon]').forEach(function(btn) {
        btn.addEventListener('click', function() {
            anonymizeEnabled = btn.dataset.anon === 'true';
            document.querySelectorAll('#anonymize-toggle .pf-v5-c-toggle-group__button').forEach(function(b) { b.classList.remove('pf-m-selected'); });
            btn.classList.add('pf-m-selected');
            document.getElementById('anon-options').style.display = anonymizeEnabled ? 'flex' : 'none';
        });
    });

    // --- Since toggle ---
    let sinceEnabled = false;
    const sinceSelect = document.getElementById('since-select');
    document.querySelectorAll('#since-toggle [data-since-enabled]').forEach(function(btn) {
        btn.addEventListener('click', function() {
            sinceEnabled = btn.dataset.sinceEnabled === 'true';
            document.querySelectorAll('#since-toggle .pf-v5-c-toggle-group__button').forEach(function(b) { b.classList.remove('pf-m-selected'); });
            btn.classList.add('pf-m-selected');
            sinceSelect.disabled = !sinceEnabled;
        });
    });

    // --- Start gather ---
    startBtn.addEventListener('click', startCustomGather);

    async function startCustomGather() {
        const namespaces = Array.from(selectedNamespaces);
        if (namespaces.length === 0) {
            showToast('No namespaces selected', 'Select at least one namespace.', 'danger');
            return;
        }
        const resourceTypes = [];
        document.querySelectorAll('.rt-check:checked').forEach(function(cb) {
            resourceTypes.push(cb.value);
        });

        const anonOpts = {
            ips: document.getElementById('anon-ips').checked,
            macs: document.getElementById('anon-macs').checked,
            domains: document.getElementById('anon-domains').checked,
            services: document.getElementById('anon-services').checked
        };
        const anonymize = anonymizeEnabled && (anonOpts.ips || anonOpts.macs || anonOpts.domains || anonOpts.services);
        const since = sinceEnabled ? sinceSelect.value : '';

        // Check if a remote cluster is selected
        const clusterSelect = document.getElementById('target-cluster');
        const targetCluster = clusterSelect ? clusterSelect.value : 'local';

        startBtn.disabled = true;
        startBtn.textContent = 'Starting...';
        try {
            var url = '/api/support/gather';
            var body = {
                type: 'custom',
                namespaces: namespaces,
                resourceTypes: resourceTypes,
                includeLogs: includeLogsEnabled,
                anonymize: anonymize,
                anonOpts: anonymize ? anonOpts : {ips: false, macs: false, domains: false, services: false},
                since: since
            };

            if (targetCluster !== 'local') {
                url = '/api/acm/gather';
                body = {
                    clusterName: targetCluster,
                    gatherTypes: ['custom'],
                    namespaces: namespaces,
                    resourceTypes: resourceTypes,
                    includeLogs: includeLogsEnabled,
                    anonymize: anonymize,
                    anonOpts: anonymize ? anonOpts : {ips: false, macs: false, domains: false, services: false},
                    since: since
                };
            }

            const res = await fetch(url, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(body)
            });
            const data = await res.json();
            if (data.error) {
                showToast('Error', data.error, 'danger');
                return;
            }
            activeJobs[data.id] = targetCluster !== 'local' ? 'remote' : 'local';
            addJobCard(data.id, namespaces.length);
            startPolling();
            var msg = targetCluster !== 'local'
                ? 'Remote gather started on ' + targetCluster
                : namespaces.length + ' namespace(s) being collected.';
            showToast('Custom Gather started', msg, 'success', data.id);
        } catch (e) {
            showToast('Failed to start gather', e.message, 'danger');
        } finally {
            startBtn.disabled = selectedNamespaces.size === 0;
            startBtn.textContent = 'Start Custom Gather';
        }
    }

    // --- Job UI (simplified from support.js) ---
    window.stopJob = async function(jobId) {
        if (!confirm('Stop this custom gather job?')) return;
        const btn = document.getElementById('stop-' + jobId);
        if (btn) { btn.disabled = true; btn.textContent = 'Stopping...'; }
        try {
            await fetch('/api/support/gather/' + encodeURIComponent(jobId) + '/stop', { method: 'POST' });
        } catch (e) { /* ignore */ }
    };

    function addJobCard(id, nsCount) {
        jobsEmpty.classList.add('hidden');
        if (document.getElementById('job-' + id)) return;
        const safeId = escapeHtml(id);
        const card = document.createElement('div');
        card.id = 'job-' + id;
        card.className = 'pf-v5-c-card pf-v5-u-mb-md';
        card.innerHTML = '<div class="pf-v5-c-card__title"><div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center"><div class="pf-v5-l-flex pf-m-gap-sm pf-m-align-items-center"><h3 class="pf-v5-c-card__title-text">Custom Gather (' + nsCount + ' ns)</h3><span class="pf-v5-c-label pf-m-blue" id="status-' + safeId + '"><span class="pf-v5-c-label__content">Running</span></span><span class="pf-v5-u-font-size-sm pf-v5-u-color-200" id="elapsed-' + safeId + '"></span></div><div id="actions-' + safeId + '"><button class="pf-v5-c-button pf-m-danger pf-m-small" id="stop-' + safeId + '" onclick="stopJob(\'' + safeId + '\')">Stop</button></div></div></div><div class="pf-v5-c-card__body"><div class="pf-v5-u-mb-sm pf-v5-u-font-size-sm" id="step-label-' + safeId + '">Initializing...</div><div class="pf-v5-c-progress pf-v5-u-mb-md" id="progress-' + safeId + '"><div class="pf-v5-c-progress__description" id="progress-text-' + safeId + '"></div><div class="pf-v5-c-progress__bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0"><div class="pf-v5-c-progress__indicator" id="progress-bar-' + safeId + '" style="width:0%;transition:width 0.5s ease;"><span class="pf-v5-c-progress__measure" id="progress-pct-' + safeId + '"></span></div></div></div><pre class="pf-v5-u-font-size-xs log-terminal" id="log-' + safeId + '" style="max-height:300px;overflow-y:auto;"></pre></div>';
        jobsContainer.prepend(card);
    }

    function formatElapsed(startedAt) {
        const secs = Math.floor((new Date() - new Date(startedAt)) / 1000);
        const m = Math.floor(secs / 60);
        const s = secs % 60;
        return m > 0 ? m + 'm ' + s + 's' : s + 's';
    }

    function startPolling() {
        if (pollInterval) return;
        pollInterval = setInterval(pollJobs, 2000);
        pollJobs();
    }

    async function pollJobs() {
        const ids = Object.keys(activeJobs);
        if (ids.length === 0) { clearInterval(pollInterval); pollInterval = null; return; }
        for (const id of ids) {
            try {
                var base = activeJobs[id] === 'remote' ? '/api/acm/gather/' : '/api/support/gather/';
                const res = await fetch(base + encodeURIComponent(id));
                const job = await res.json();
                if (job.error && !job.status) continue;
                updateJobUI(job);
                if (job.status === 'complete' || job.status === 'failed') delete activeJobs[id];
            } catch (e) { /* ignore */ }
        }
    }

    function updateJobUI(job) {
        const logEl = document.getElementById('log-' + job.id);
        const statusEl = document.getElementById('status-' + job.id);
        const progressBar = document.getElementById('progress-bar-' + job.id);
        const progressPct = document.getElementById('progress-pct-' + job.id);
        const progressText = document.getElementById('progress-text-' + job.id);
        const stepLabel = document.getElementById('step-label-' + job.id);
        const elapsedEl = document.getElementById('elapsed-' + job.id);
        const actionsEl = document.getElementById('actions-' + job.id);
        const progressEl = document.getElementById('progress-' + job.id);
        if (!logEl) return;

        if (job.logOutput) {
            logEl.textContent = job.logOutput;
            logEl.scrollTop = logEl.scrollHeight;
        }
        if (elapsedEl && job.startedAt) elapsedEl.textContent = formatElapsed(job.startedAt);
        if (job.totalSteps > 0 && progressBar) {
            const pct = Math.round((job.step / job.totalSteps) * 100);
            progressBar.style.width = pct + '%';
            progressPct.textContent = pct + '%';
            progressText.textContent = 'Step ' + job.step + ' of ' + job.totalSteps;
        }
        if (stepLabel && job.stepLabel) stepLabel.textContent = job.stepLabel;

        if (job.status === 'complete') {
            statusEl.className = 'pf-v5-c-label pf-m-green';
            statusEl.innerHTML = '<span class="pf-v5-c-label__content">Complete</span>';
            if (stepLabel) stepLabel.textContent = job.warning || 'Archive ready for download';
            if (progressBar) { progressBar.style.width = '100%'; progressPct.textContent = '100%'; }
            if (progressText) progressText.textContent = 'Done';
            var dlBase = job.id.indexOf('remote-') === 0 ? '/api/acm/gather/' : '/api/support/gather/';
            actionsEl.innerHTML = '<a href="' + dlBase + encodeURIComponent(job.id) + '/download" class="pf-v5-c-button pf-m-primary pf-m-small">Download</a>';
        } else if (job.status === 'failed') {
            statusEl.className = 'pf-v5-c-label pf-m-red';
            const isStopped = job.error && job.error.includes('Stopped by user');
            statusEl.innerHTML = '<span class="pf-v5-c-label__content">' + (isStopped ? 'Stopped' : 'Failed') + '</span>';
            if (progressEl) progressEl.classList.add('hidden');
            actionsEl.innerHTML = '';
            if (stepLabel) stepLabel.textContent = isStopped ? 'Stopped by user' : (job.error || 'Failed');
        }
    }

    // Load existing jobs on page load
    async function loadExistingJobs() {
        try {
            const res = await fetch('/api/support/jobs');
            const jobs = await res.json();
            if (!jobs || jobs.length === 0) return;
            // Show only custom jobs
            const customJobs = jobs.filter(function(j) { return j.type === 'custom'; });
            if (customJobs.length === 0) return;
            jobsEmpty.classList.add('hidden');
            customJobs.sort(function(a, b) { return new Date(b.startedAt) - new Date(a.startedAt); });
            for (const job of customJobs) {
                addJobCard(job.id, 0);
                updateJobUI(job);
                if (job.status === 'running') activeJobs[job.id] = 'local';
            }
            if (Object.keys(activeJobs).length > 0) startPolling();
        } catch (e) { /* ignore */ }
    }

    // Show ACM nav and cluster selector if available
    fetch('/api/support/capabilities').then(function(r) { return r.json(); }).then(function(caps) {
        if (caps.acm) {
            var acmNav = document.getElementById('acm-nav');
            if (acmNav) acmNav.style.display = '';

            // Populate cluster selector
            fetch('/api/acm/clusters').then(function(r) { return r.json(); }).then(function(clusters) {
                if (!clusters || clusters.length === 0) return;
                var selectorCard = document.getElementById('cluster-selector-card');
                var select = document.getElementById('target-cluster');
                if (selectorCard) selectorCard.style.display = '';
                for (var i = 0; i < clusters.length; i++) {
                    if (clusters[i].status === 'Available') {
                        var opt = document.createElement('option');
                        opt.value = clusters[i].name;
                        opt.textContent = clusters[i].name + (clusters[i].ocpVersion ? ' (' + clusters[i].ocpVersion + ')' : '');
                        select.appendChild(opt);
                    }
                }
                select.addEventListener('change', function() {
                    loadNamespaces(select.value);
                });
            }).catch(function() {});
        }
    }).catch(function() {});

    loadNamespaces();
    loadExistingJobs();
})();
