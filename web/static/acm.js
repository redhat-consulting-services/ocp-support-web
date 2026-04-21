(function() {
    'use strict';

    var activeJobs = {};
    var pollInterval = null;
    var clusterPollInterval = null;
    var modalCluster = '';
    var modalSelectedTypes = new Set();
    var clusterOperators = {}; // clusterName → [{type, label, version}]
    var clustersLoaded = false;
    var hasDeploying = false;

    loadClusters();
    loadExistingJobs();

    async function loadExistingJobs() {
        try {
            var res = await fetch('/api/acm/gather/jobs');
            if (!res.ok) return;
            var jobs = await res.json();
            if (!jobs || jobs.length === 0) return;

            for (var i = 0; i < jobs.length; i++) {
                var job = jobs[i];
                var safeId = job.id.replace(/[^a-zA-Z0-9-]/g, '-');
                if (document.getElementById('job-card-' + safeId)) continue;

                createJobCard(job.id, job.clusterName, job.gatherType || 'Default');
                updateJobUI(job);

                if (job.status !== 'complete' && job.status !== 'failed') {
                    activeJobs[job.id] = true;
                }
            }

            if (Object.keys(activeJobs).length > 0) {
                startPolling();
            }
        } catch (e) {
            // ignore — jobs will appear when started
        }
    }

    // Anon toggle
    document.getElementById('modal-anon-toggle').addEventListener('change', function() {
        document.getElementById('modal-anon-options').classList.toggle('hidden', !this.checked);
    });

    async function loadClusters() {
        try {
            var res = await fetch('/api/acm/clusters');
            if (!res.ok) throw new Error('Failed to load clusters');
            var clusters = await res.json();

            document.getElementById('clusters-loading').classList.add('hidden');

            if (!clusters || clusters.length === 0) {
                document.getElementById('clusters-empty').classList.remove('hidden');
                return;
            }

            var table = document.getElementById('clusters-table');
            var tbody = document.getElementById('clusters-tbody');
            table.style.display = '';
            tbody.innerHTML = '';

            hasDeploying = false;

            for (var i = 0; i < clusters.length; i++) {
                var c = clusters[i];
                var row = document.createElement('tr');

                var statusBadge = '';
                if (c.status === 'Available') {
                    statusBadge = '<span class="pf-v5-c-label pf-m-green"><span class="pf-v5-c-label__content">Available</span></span>';
                } else if (c.status === 'Unavailable') {
                    statusBadge = '<span class="pf-v5-c-label pf-m-red"><span class="pf-v5-c-label__content">Unavailable</span></span>';
                } else {
                    statusBadge = '<span class="pf-v5-c-label"><span class="pf-v5-c-label__content">Unknown</span></span>';
                }

                var platformDisplay = c.platform || '-';
                if (c.vendor && c.vendor !== 'OpenShift' && c.platform) {
                    platformDisplay = c.platform + ' (' + escapeHtml(c.vendor) + ')';
                }

                var agentBadge = '';
                if (c.agentReady) {
                    agentBadge = '<span class="pf-v5-c-label pf-m-green pf-m-compact"><span class="pf-v5-c-label__content">Ready</span></span>';
                } else if (c.agentDeployed && c.status === 'Available') {
                    agentBadge = '<span class="pf-v5-c-label pf-m-blue pf-m-compact"><span class="pf-v5-c-label__content">Deploying</span></span>';
                    hasDeploying = true;
                } else if (c.status === 'Available') {
                    agentBadge = '<button class="pf-v5-c-button pf-m-link pf-m-small install-agent-btn" data-cluster="' + escapeAttr(c.name) + '">Install Agent</button>';
                } else {
                    agentBadge = '<span class="pf-v5-c-label pf-m-compact"><span class="pf-v5-c-label__content">-</span></span>';
                }

                var gatherCell = '';
                if (c.status === 'Available' && c.agentReady) {
                    gatherCell = '<button class="pf-v5-c-button pf-m-primary pf-m-small start-gather-btn" data-cluster="' + escapeAttr(c.name) + '">Start Gather</button>';
                } else if (c.agentDeployed && c.status === 'Available') {
                    gatherCell = '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent deploying...</span>';
                } else if (c.status === 'Available') {
                    gatherCell = '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent not installed</span>';
                } else {
                    gatherCell = '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">Cluster unavailable</span>';
                }

                var agentActionsCell = '';
                if (c.status === 'Available' && c.agentDeployed) {
                    var kebabId = 'kebab-' + escapeAttr(c.name);
                    agentActionsCell =
                        '<div class="pf-v5-c-dropdown" id="' + kebabId + '">' +
                            '<button class="pf-v5-c-dropdown__toggle pf-m-plain kebab-toggle" data-kebab="' + kebabId + '" aria-label="Actions">' +
                                '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 192 512"><path d="M96 184c39.8 0 72 32.2 72 72s-32.2 72-72 72-72-32.2-72-72 32.2-72 72-72zM24 80c0 39.8 32.2 72 72 72s72-32.2 72-72S135.8 8 96 8 24 40.2 24 80zm0 352c0 39.8 32.2 72 72 72s72-32.2 72-72-32.2-72-72-72-72 32.2-72 72z"/></svg>' +
                            '</button>' +
                            '<ul class="pf-v5-c-dropdown__menu" style="display:none;position:absolute;right:0;z-index:100;min-width:180px;" id="' + kebabId + '-menu">' +
                                '<li><button class="pf-v5-c-dropdown__menu-item refresh-ops-btn" data-cluster="' + escapeAttr(c.name) + '">Refresh Operators</button></li>' +
                                '<li><button class="pf-v5-c-dropdown__menu-item update-agent-btn" data-cluster="' + escapeAttr(c.name) + '" style="display:none;">Update Agent</button></li>' +
                                '<li><button class="pf-v5-c-dropdown__menu-item reinstall-agent-btn" data-cluster="' + escapeAttr(c.name) + '">Reinstall Agent</button></li>' +
                                '<li><button class="pf-v5-c-dropdown__menu-item remove-agent-btn" data-cluster="' + escapeAttr(c.name) + '">Remove Agent</button></li>' +
                            '</ul>' +
                        '</div>';
                }

                row.innerHTML =
                    '<td>' + escapeHtml(c.name) + '</td>' +
                    '<td>' + statusBadge + '</td>' +
                    '<td>' + escapeHtml(c.ocpVersion || '-') + '</td>' +
                    '<td>' + escapeHtml(platformDisplay) + '</td>' +
                    '<td>' + agentBadge + '</td>' +
                    '<td class="agent-version-cell" data-cluster="' + escapeAttr(c.name) + '">' + (c.agentReady ? '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">loading...</span>' : '-') + '</td>' +
                    '<td>' + gatherCell + '</td>' +
                    '<td>' + agentActionsCell + '</td>';

                tbody.appendChild(row);
            }

            bindClusterButtons();
            clustersLoaded = true;

            // Fetch agent version for ready clusters
            for (var v = 0; v < clusters.length; v++) {
                if (clusters[v].agentReady) {
                    fetchAgentVersion(clusters[v].name);
                }
            }

            // Auto-refresh while any agents are still deploying
            if (hasDeploying && !clusterPollInterval) {
                clusterPollInterval = setInterval(function() {
                    loadClusters();
                }, 10000);
            } else if (!hasDeploying && clusterPollInterval) {
                clearInterval(clusterPollInterval);
                clusterPollInterval = null;
            }
        } catch (e) {
            if (!clustersLoaded) {
                document.getElementById('clusters-loading').classList.add('hidden');
                var errEl = document.getElementById('clusters-error');
                errEl.textContent = 'Failed to load managed clusters: ' + e.message;
                errEl.classList.remove('hidden');
            }
        }
    }

    async function fetchAgentVersion(clusterName) {
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/version');
            if (!res.ok) throw new Error('failed');
            var data = await res.json();
            var agentVer = data.version || '';
            var cell = document.querySelector('.agent-version-cell[data-cluster="' + clusterName + '"]');
            if (cell) {
                cell.textContent = agentVer || '-';
            }
            // Show Update button if agent version differs from hub version
            var updateBtn = document.querySelector('.update-agent-btn[data-cluster="' + clusterName + '"]');
            if (updateBtn && typeof hubVersion !== 'undefined' && hubVersion && agentVer) {
                if (agentVer !== hubVersion) {
                    updateBtn.style.display = '';
                    updateBtn.title = 'Update agent from ' + agentVer + ' to ' + hubVersion;
                }
            }
        } catch (e) {
            var cell = document.querySelector('.agent-version-cell[data-cluster="' + clusterName + '"]');
            if (cell) cell.textContent = '-';
        }
    }

    function bindClusterButtons() {
        // Bind start gather buttons
        var btns = document.querySelectorAll('.start-gather-btn');
        for (var b = 0; b < btns.length; b++) {
            btns[b].addEventListener('click', function() {
                openGatherModal(this.dataset.cluster);
            });
        }

        // Bind kebab toggles
        var kebabs = document.querySelectorAll('.kebab-toggle');
        for (var k = 0; k < kebabs.length; k++) {
            kebabs[k].addEventListener('click', function(e) {
                e.stopPropagation();
                var menuId = this.dataset.kebab + '-menu';
                var menu = document.getElementById(menuId);
                if (!menu) return;
                // Close other open kebab menus
                var kebabMenus = document.querySelectorAll('[id^="kebab-"][id$="-menu"]');
                for (var m = 0; m < kebabMenus.length; m++) {
                    if (kebabMenus[m].id !== menuId) kebabMenus[m].style.display = 'none';
                }
                menu.style.display = menu.style.display === 'none' ? 'block' : 'none';
            });
        }

        // Close kebab menus when clicking elsewhere
        document.addEventListener('click', function() {
            var kebabMenus = document.querySelectorAll('[id^="kebab-"][id$="-menu"]');
            for (var m = 0; m < kebabMenus.length; m++) {
                kebabMenus[m].style.display = 'none';
            }
        });

        // Bind refresh operators buttons
        var refreshBtns = document.querySelectorAll('.refresh-ops-btn');
        for (var rf = 0; rf < refreshBtns.length; rf++) {
            refreshBtns[rf].addEventListener('click', function() {
                refreshOperators(this.dataset.cluster);
            });
        }

        // Bind update agent buttons
        var updateBtns = document.querySelectorAll('.update-agent-btn');
        for (var ub = 0; ub < updateBtns.length; ub++) {
            updateBtns[ub].addEventListener('click', function() {
                updateAgent(this.dataset.cluster, this);
            });
        }

        // Bind reinstall agent buttons
        var reinstallBtns = document.querySelectorAll('.reinstall-agent-btn');
        for (var ri = 0; ri < reinstallBtns.length; ri++) {
            reinstallBtns[ri].addEventListener('click', function() {
                reinstallAgent(this.dataset.cluster, this);
            });
        }

        // Bind remove agent buttons
        var removeBtns = document.querySelectorAll('.remove-agent-btn');
        for (var rmb = 0; rmb < removeBtns.length; rmb++) {
            removeBtns[rmb].addEventListener('click', function() {
                removeAgent(this.dataset.cluster, this);
            });
        }

        // Bind install agent buttons
        var installBtns = document.querySelectorAll('.install-agent-btn');
        for (var ib = 0; ib < installBtns.length; ib++) {
            installBtns[ib].addEventListener('click', function() {
                installAgent(this.dataset.cluster, this);
            });
        }
    }

    async function openGatherModal(clusterName) {
        modalCluster = clusterName;
        modalSelectedTypes = new Set();
        document.getElementById('modal-cluster-name').textContent = clusterName;
        document.getElementById('modal-since').value = '';
        document.getElementById('modal-anon-toggle').checked = false;
        document.getElementById('modal-anon-options').classList.add('hidden');

        var container = document.getElementById('modal-gather-types');
        container.innerHTML = '';

        document.getElementById('gather-modal-backdrop').style.display = '';

        // Load operators from agent
        var loadingEl = document.getElementById('modal-operators-loading');
        loadingEl.classList.remove('hidden');
        document.getElementById('modal-start-btn').disabled = true;

        try {
            var ops = clusterOperators[clusterName];
            if (!ops) {
                var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/operators');
                if (res.ok) {
                    ops = await res.json();
                    clusterOperators[clusterName] = ops;
                }
            }

            loadingEl.classList.add('hidden');
            document.getElementById('modal-start-btn').disabled = false;

            if (ops && ops.length > 0) {
                for (var i = 0; i < ops.length; i++) {
                    var op = ops[i];
                    var card = document.createElement('div');
                    card.className = 'gather-type-card';
                    card.dataset.type = op.type;
                    var verStr = op.version ? ' (v' + escapeHtml(op.version) + ')' : '';
                    card.innerHTML =
                        '<div class="gather-type-card__title">' + escapeHtml(op.label) + '</div>' +
                        '<div class="gather-type-card__desc pf-v5-u-font-size-xs" style="opacity:0.6;">' + escapeHtml(op.type) + verStr + '</div>';
                    card.addEventListener('click', function() {
                        var t = this.dataset.type;
                        if (modalSelectedTypes.has(t)) {
                            modalSelectedTypes.delete(t);
                            this.classList.remove('selected');
                        } else {
                            modalSelectedTypes.add(t);
                            this.classList.add('selected');
                        }
                    });
                    container.appendChild(card);
                }
            } else {
                container.innerHTML = '<p class="pf-v5-u-font-size-xs pf-v5-u-color-200">No additional operators detected on this cluster. Default gather will run.</p>';
            }
        } catch (e) {
            loadingEl.classList.add('hidden');
            document.getElementById('modal-start-btn').disabled = false;
            container.innerHTML = '<p class="pf-v5-u-font-size-xs pf-v5-u-color-200">Could not detect operators. Default gather will run.</p>';
        }
    }

    window.closeGatherModal = function() {
        document.getElementById('gather-modal-backdrop').style.display = 'none';
        modalCluster = '';
        modalSelectedTypes = new Set();
    };

    window.confirmGatherStart = async function() {
        if (!modalCluster) return;
        var clusterName = modalCluster;
        var types = ['default'].concat(Array.from(modalSelectedTypes));

        // Build display label
        var typeLabel = 'Default';
        if (modalSelectedTypes.size > 0) {
            var extras = [];
            var ops = clusterOperators[clusterName] || [];
            for (var i = 0; i < ops.length; i++) {
                if (modalSelectedTypes.has(ops[i].type)) {
                    extras.push(ops[i].label);
                }
            }
            if (extras.length > 0) {
                typeLabel = 'Default + ' + extras.join(', ');
            }
        }

        var since = document.getElementById('modal-since').value;
        var anonymize = document.getElementById('modal-anon-toggle').checked;
        var anonOpts = {};
        if (anonymize) {
            anonOpts = {
                ips: document.getElementById('anon-ips').checked,
                macs: document.getElementById('anon-macs').checked,
                domains: document.getElementById('anon-domains').checked,
                services: document.getElementById('anon-services').checked,
                secrets: document.getElementById('anon-secrets').checked
            };
        }

        closeGatherModal();

        // Disable the start button for this cluster
        var btns = document.querySelectorAll('.start-gather-btn[data-cluster="' + clusterName + '"]');
        for (var b = 0; b < btns.length; b++) {
            btns[b].disabled = true;
            btns[b].innerHTML = '<span class="btn-spinner"></span>Starting...';
        }

        try {
            var res = await fetch('/api/acm/gather', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    clusterName: clusterName,
                    gatherTypes: types,
                    since: since,
                    anonymize: anonymize,
                    anonOpts: anonOpts
                })
            });
            var data = await res.json();
            if (!res.ok) {
                showToast('Failed to start gather: ' + (data.error || 'unknown error'), 'danger');
                resetGatherButton(clusterName);
                return;
            }

            activeJobs[data.id] = true;
            createJobCard(data.id, clusterName, typeLabel);
            startPolling();

            showToast('Remote gather started on ' + clusterName + ' (' + typeLabel + ')', 'success');
        } catch (e) {
            showToast('Error: ' + e.message, 'danger');
            resetGatherButton(clusterName);
        }
    };

    function resetGatherButton(clusterName) {
        var btns = document.querySelectorAll('.start-gather-btn[data-cluster="' + clusterName + '"]');
        for (var i = 0; i < btns.length; i++) {
            btns[i].disabled = false;
            btns[i].textContent = 'Start Gather';
        }
    }

    function createJobCard(id, clusterName, typeLabel) {
        var safeId = id.replace(/[^a-zA-Z0-9-]/g, '-');
        var container = document.getElementById('jobs-container');
        var emptyEl = document.getElementById('jobs-empty');
        if (emptyEl) emptyEl.classList.add('hidden');

        var card = document.createElement('div');
        card.id = 'job-card-' + safeId;
        card.className = 'pf-v5-c-card pf-v5-u-mb-md';
        card.innerHTML =
            '<div class="pf-v5-c-card__title">' +
                '<div class="pf-v5-l-flex pf-m-align-items-center pf-m-justify-content-space-between">' +
                    '<div class="pf-v5-l-flex pf-m-align-items-center pf-m-gap-sm">' +
                        '<h2 class="pf-v5-c-card__title-text">' + escapeHtml(clusterName) + ' — ' + escapeHtml(typeLabel) + '</h2>' +
                        '<span class="pf-v5-c-label pf-m-blue" id="status-' + safeId + '"><span class="pf-v5-c-label__content">gathering</span></span>' +
                    '</div>' +
                    '<div class="pf-v5-l-flex pf-m-align-items-center pf-m-gap-sm">' +
                        '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200" id="elapsed-' + safeId + '"></span>' +
                        '<div id="actions-' + safeId + '"></div>' +
                    '</div>' +
                '</div>' +
            '</div>' +
            '<div class="pf-v5-c-card__body">' +
                '<pre class="pf-v5-u-font-size-xs log-terminal" id="log-' + safeId + '" style="max-height:200px;overflow-y:auto;"></pre>' +
            '</div>';

        container.prepend(card);
    }

    function startPolling() {
        if (pollInterval) return;
        pollInterval = setInterval(pollJobs, 3000);
        pollJobs();
    }

    async function pollJobs() {
        var ids = Object.keys(activeJobs);
        if (ids.length === 0) {
            clearInterval(pollInterval);
            pollInterval = null;
            return;
        }

        for (var i = 0; i < ids.length; i++) {
            var id = ids[i];
            try {
                var res = await fetch('/api/acm/gather/' + encodeURIComponent(id));
                var job = await res.json();
                if (job.error && !job.status) continue;

                var safeId = id.replace(/[^a-zA-Z0-9-]/g, '-');
                if (!document.getElementById('job-card-' + safeId)) {
                    createJobCard(job.id, job.clusterName, job.gatherType || 'Default');
                }

                updateJobUI(job);
                if (job.status === 'complete' || job.status === 'failed') {
                    delete activeJobs[id];
                    resetGatherButton(job.clusterName);
                }
            } catch (e) {
                // ignore transient errors
            }
        }
    }

    function statusLabel(status) {
        var labels = {
            'gathering': { cls: 'pf-m-blue', text: 'Gathering' },
            'downloading': { cls: 'pf-m-blue', text: 'Downloading' },
            'anonymizing': { cls: 'pf-m-blue', text: 'Anonymizing' },
            'complete': { cls: 'pf-m-green', text: 'Complete' },
            'failed': { cls: 'pf-m-red', text: 'Failed' }
        };
        return labels[status] || { cls: 'pf-m-blue', text: status };
    }

    function updateJobUI(job) {
        var safeId = job.id.replace(/[^a-zA-Z0-9-]/g, '-');
        var logEl = document.getElementById('log-' + safeId);
        var statusEl = document.getElementById('status-' + safeId);
        var elapsedEl = document.getElementById('elapsed-' + safeId);
        var actionsEl = document.getElementById('actions-' + safeId);
        if (!logEl) return;

        if (job.logOutput) {
            logEl.textContent = job.logOutput;
            logEl.scrollTop = logEl.scrollHeight;
        }

        if (statusEl) {
            var sl = statusLabel(job.status);
            statusEl.className = 'pf-v5-c-label ' + sl.cls;
            statusEl.querySelector('.pf-v5-c-label__content').textContent = sl.text;
        }

        if (elapsedEl && job.startedAt) {
            elapsedEl.textContent = formatElapsed(job.startedAt);
        }

        if (actionsEl && (job.status === 'complete' || job.status === 'failed')) {
            var html = '';
            if (job.status === 'complete' && job.fileName) {
                html += '<a class="pf-v5-c-button pf-m-primary pf-m-small" href="/api/acm/gather/' + encodeURIComponent(job.id) + '/download" style="margin-right:4px;">Download</a>';
            }
            html += '<button class="pf-v5-c-button pf-m-danger pf-m-small" onclick="deleteRemoteJob(\'' + escapeAttr(job.id) + '\')">Delete</button>';
            actionsEl.innerHTML = html;
        }
    }

    window.deleteRemoteJob = async function(jobId) {
        try {
            var res = await fetch('/api/acm/gather/' + encodeURIComponent(jobId), { method: 'DELETE' });
            if (res.ok) {
                var safeId = jobId.replace(/[^a-zA-Z0-9-]/g, '-');
                var card = document.getElementById('job-card-' + safeId);
                if (card) card.remove();
                showToast('Remote gather job deleted', 'success');

                var container = document.getElementById('jobs-container');
                if (container && container.children.length === 0) {
                    document.getElementById('jobs-empty').classList.remove('hidden');
                }
            }
        } catch (e) {
            showToast('Failed to delete: ' + e.message, 'danger');
        }
    };

    async function refreshOperators(clusterName) {
        // Clear cached operators so next modal open fetches fresh data
        delete clusterOperators[clusterName];
        showToast('Refreshing operators for ' + clusterName + '...', 'success');
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/operators');
            if (res.ok) {
                var ops = await res.json();
                clusterOperators[clusterName] = ops;
                showToast('Operators refreshed: ' + (ops.length || 0) + ' detected on ' + clusterName, 'success');
            } else {
                showToast('Failed to refresh operators', 'danger');
            }
        } catch (e) {
            showToast('Error refreshing operators: ' + e.message, 'danger');
        }
    }

    async function updateAgent(clusterName, btn) {
        if (!await pfConfirm('Update Agent', 'Update the agent on ' + clusterName + ' to version ' + hubVersion + '? This will briefly interrupt any running gather.')) return;
        btn.disabled = true;
        btn.textContent = 'Updating...';
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/agent/redeploy', { method: 'POST' });
            var data = await res.json();
            if (!res.ok) {
                showToast('Failed to update agent: ' + (data.error || 'unknown error'), 'danger');
            } else {
                showToast('Agent updated on ' + clusterName, 'success');
                btn.style.display = 'none';
                // Refresh version after a delay
                setTimeout(function() { fetchAgentVersion(clusterName); }, 15000);
            }
        } catch (e) {
            showToast('Error: ' + e.message, 'danger');
        }
        btn.disabled = false;
        btn.textContent = 'Update';
    }

    async function reinstallAgent(clusterName, btn) {
        if (!await pfConfirm('Reinstall Agent', 'Reinstall the agent on ' + clusterName + '? This will remove and redeploy all agent resources.', { danger: true })) return;
        btn.disabled = true;
        btn.textContent = 'Reinstalling...';
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/agent/redeploy', { method: 'POST' });
            var data = await res.json();
            if (!res.ok) {
                showToast('Failed to reinstall agent: ' + (data.error || 'unknown error'), 'danger');
            } else {
                showToast('Agent reinstalled on ' + clusterName, 'success');
                delete clusterOperators[clusterName];
                setTimeout(function() { loadClusters(); }, 10000);
            }
        } catch (e) {
            showToast('Error: ' + e.message, 'danger');
        }
        btn.disabled = false;
        btn.textContent = 'Reinstall Agent';
    }

    async function removeAgent(clusterName, btn) {
        if (!await pfConfirm('Remove Agent', 'Remove the agent from ' + clusterName + '? This will delete all agent resources on the remote cluster.', { danger: true })) return;
        btn.disabled = true;
        btn.textContent = 'Removing...';
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/agent', { method: 'DELETE' });
            var data = await res.json();
            if (!res.ok) {
                showToast('Failed to remove agent: ' + (data.error || 'unknown error'), 'danger');
            } else {
                showToast('Agent removed from ' + clusterName, 'success');
                var row = btn.closest('tr');
                if (row) {
                    var cells = row.querySelectorAll('td');
                    if (cells[4]) {
                        var installBtn = document.createElement('button');
                        installBtn.className = 'pf-v5-c-button pf-m-link pf-m-small install-agent-btn';
                        installBtn.dataset.cluster = clusterName;
                        installBtn.textContent = 'Install Agent';
                        installBtn.addEventListener('click', function() { installAgent(clusterName, this); });
                        cells[4].innerHTML = '';
                        cells[4].appendChild(installBtn);
                    }
                    if (cells[5]) cells[5].textContent = '-';
                    if (cells[6]) cells[6].innerHTML = '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent not installed</span>';
                }
            }
        } catch (e) {
            showToast('Error: ' + e.message, 'danger');
        }
        btn.disabled = false;
        btn.textContent = 'Remove Agent';
    }

    async function installAgent(clusterName, btn) {
        btn.disabled = true;
        btn.textContent = 'Installing...';
        try {
            var res = await fetch('/api/acm/clusters/' + encodeURIComponent(clusterName) + '/agent', { method: 'POST' });
            var data = await res.json();
            if (!res.ok) {
                showToast('Failed to install agent: ' + (data.error || 'unknown error'), 'danger');
                btn.disabled = false;
                btn.textContent = 'Install Agent';
            } else {
                showToast('Agent deploying to ' + clusterName, 'success');
                var row = btn.closest('tr');
                if (row) {
                    var cells = row.querySelectorAll('td');
                    if (cells[4]) cells[4].innerHTML = '<span class="pf-v5-c-label pf-m-blue pf-m-compact"><span class="pf-v5-c-label__content">Deploying</span></span>';
                    if (cells[6]) cells[6].innerHTML = '<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">Agent deploying...</span>';
                }
                hasDeploying = true;
                if (!clusterPollInterval) {
                    clusterPollInterval = setInterval(function() { loadClusters(); }, 10000);
                }
            }
        } catch (e) {
            showToast('Error: ' + e.message, 'danger');
            btn.disabled = false;
            btn.textContent = 'Install Agent';
        }
    }

    function formatElapsed(startedAt) {
        var start = new Date(startedAt);
        var now = new Date();
        var secs = Math.floor((now - start) / 1000);
        if (secs < 60) return secs + 's';
        var mins = Math.floor(secs / 60);
        secs = secs % 60;
        if (mins < 60) return mins + 'm ' + secs + 's';
        var hrs = Math.floor(mins / 60);
        mins = mins % 60;
        return hrs + 'h ' + mins + 'm';
    }

    function showToast(message, variant) {
        var container = document.getElementById('toast-container');
        if (!container) return;
        variant = variant || 'info';
        var icons = {
            success: '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 512 512"><path d="M504 256c0 136.967-111.033 248-248 248S8 392.967 8 256 119.033 8 256 8s248 111.033 248 248zM227.314 387.314l184-184c6.248-6.248 6.248-16.379 0-22.627l-22.627-22.627c-6.248-6.249-16.379-6.249-22.628 0L216 308.118l-70.059-70.059c-6.248-6.248-16.379-6.248-22.628 0l-22.627 22.627c-6.248 6.248-6.248 16.379 0 22.627l104 104c6.249 6.249 16.379 6.249 22.628.001z"/></svg>',
            danger: '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 512 512"><path d="M504 256c0 136.997-111.043 248-248 248S8 392.997 8 256C8 119.083 119.043 8 256 8s248 111.083 248 248zm-248 50c-25.405 0-46 20.595-46 46s20.595 46 46 46 46-20.595 46-46-20.595-46-46-46zm-43.673-165.346l7.418 136c.347 6.364 5.609 11.346 11.982 11.346h48.546c6.373 0 11.635-4.982 11.982-11.346l7.418-136c.375-6.874-5.098-12.654-11.982-12.654h-63.383c-6.884 0-12.356 5.78-11.981 12.654z"/></svg>',
            info: '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 512 512"><path d="M256 8C119.043 8 8 119.083 8 256c0 136.997 111.043 248 248 248s248-111.003 248-248C504 119.083 392.957 8 256 8zm0 110c23.196 0 42 18.804 42 42s-18.804 42-42 42-42-18.804-42-42 18.804-42 42-42zm56 254c0 6.627-5.373 12-12 12h-88c-6.627 0-12-5.373-12-12v-24c0-6.627 5.373-12 12-12h12v-64h-12c-6.627 0-12-5.373-12-12v-24c0-6.627 5.373-12 12-12h64c6.627 0 12 5.373 12 12v100h12c6.627 0 12 5.373 12 12v24z"/></svg>'
        };
        var li = document.createElement('li');
        li.className = 'pf-v5-c-alert-group__item';
        li.innerHTML = '<div class="pf-v5-c-alert pf-m-' + variant + '" aria-label="' + escapeAttr(message) + '">' +
            '<div class="pf-v5-c-alert__icon">' + (icons[variant] || icons.info) + '</div>' +
            '<p class="pf-v5-c-alert__title">' + escapeHtml(message) + '</p>' +
            '<div class="pf-v5-c-alert__action"><button class="pf-v5-c-button pf-m-plain" type="button" aria-label="Close" onclick="this.closest(\'.pf-v5-c-alert-group__item\').remove()">' +
            '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 352 512"><path d="M242.72 256l100.07-100.07c12.28-12.28 12.28-32.19 0-44.48l-22.24-22.24c-12.28-12.28-32.19-12.28-44.48 0L176 189.28 75.93 89.21c-12.28-12.28-32.19-12.28-44.48 0L9.21 111.45c-12.28 12.29-12.28 32.2 0 44.48L109.28 256 9.21 356.07c-12.28 12.28-12.28 32.19 0 44.48l22.24 22.24c12.28 12.28 32.2 12.28 44.48 0L176 322.72l100.07 100.07c12.28 12.28 32.2 12.28 44.48 0l22.24-22.24c12.28-12.28 12.28-32.19 0-44.48L242.72 256z"/></svg>' +
            '</button></div></div>';
        container.appendChild(li);
        setTimeout(function() { if (li.parentNode) li.remove(); }, 6000);
    }

    function escapeHtml(str) {
        if (!str) return '';
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function escapeAttr(str) {
        return escapeHtml(str).replace(/'/g, '&#39;');
    }
})();
