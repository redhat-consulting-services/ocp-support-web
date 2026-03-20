(function() {
    // --- Cluster Health ---
    async function loadClusterHealth() {
        try {
            const resp = await fetch('/api/status/cluster');
            const data = await resp.json();
            if (!resp.ok) throw new Error(data.error || 'request failed');
            renderCluster(data);
            renderControlPlane(data.controlPlane);
            renderOperators(data.operators);
            renderODF(data.odf);
        } catch (e) {
            document.getElementById('cluster-body').innerHTML = errorMsg(e.message);
        }
    }

    function statusLabel(ok, degraded) {
        if (degraded) return '<span class="pf-v5-c-label pf-m-red"><span class="pf-v5-c-label__content">Degraded</span></span>';
        if (ok) return '<span class="pf-v5-c-label pf-m-green"><span class="pf-v5-c-label__content">Available</span></span>';
        return '<span class="pf-v5-c-label pf-m-orange"><span class="pf-v5-c-label__content">Unavailable</span></span>';
    }

    function renderCluster(data) {
        const statusColor = data.status === 'Available' ? 'green' : data.status === 'Degraded' ? 'red' : 'orange';
        const encColor = data.etcdEncryption && data.etcdEncryption.startsWith('Encrypted') ? 'green' : data.etcdEncryption === 'Unknown' ? 'orange' : 'red';
        document.getElementById('cluster-body').innerHTML = `
            <div class="pf-v5-l-flex pf-m-column pf-m-gap-sm">
                <div class="pf-v5-l-flex pf-m-justify-content-space-between">
                    <span class="pf-v5-u-font-weight-bold">Version</span>
                    <span>${escapeHtml(data.version)}</span>
                </div>
                <div class="pf-v5-l-flex pf-m-justify-content-space-between">
                    <span class="pf-v5-u-font-weight-bold">Status</span>
                    <span class="pf-v5-c-label pf-m-${statusColor}"><span class="pf-v5-c-label__content">${escapeHtml(data.status)}</span></span>
                </div>
                <div class="pf-v5-l-flex pf-m-justify-content-space-between">
                    <span class="pf-v5-u-font-weight-bold">Etcd Encryption</span>
                    <span class="pf-v5-c-label pf-m-${encColor}"><span class="pf-v5-c-label__content">${escapeHtml(data.etcdEncryption || 'Unknown')}</span></span>
                </div>
            </div>`;
    }

    function renderControlPlane(ops) {
        if (!ops || ops.length === 0) {
            document.getElementById('cp-body').innerHTML = '<span class="pf-v5-u-color-200">No data</span>';
            return;
        }
        let html = `<div class="pf-v5-l-flex pf-m-column pf-m-gap-sm">`;
        for (const op of ops) {
            html += `<div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center">
                <span class="pf-v5-u-font-size-sm">${escapeHtml(op.name)}</span>
                ${statusLabel(op.available, op.degraded)}
            </div>`;
        }
        html += '</div>';
        document.getElementById('cp-body').innerHTML = html;
    }

    function renderOperators(ops) {
        if (!ops || ops.length === 0) {
            document.getElementById('ops-body').innerHTML = '<span class="pf-v5-u-color-200">No data</span>';
            return;
        }
        let html = `<div class="pf-v5-l-flex pf-m-column pf-m-gap-sm">`;
        for (const op of ops) {
            html += `<div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center">
                <span class="pf-v5-u-font-size-sm">${escapeHtml(op.name)}</span>
                ${statusLabel(op.available, op.degraded)}
            </div>`;
            if (op.degraded && op.message) {
                html += `<div class="pf-v5-u-font-size-xs pf-v5-u-danger-color-100" style="margin-top:-4px;word-break:break-word;">${escapeHtml(op.message.substring(0, 200))}</div>`;
            }
        }
        html += '</div>';
        document.getElementById('ops-body').innerHTML = html;
    }

    function renderODF(odf) {
        if (!odf || !odf.installed) {
            document.getElementById('odf-body').innerHTML = '<span class="pf-v5-u-color-200">ODF not installed</span>';
            return;
        }
        const color = odf.phase === 'Ready' ? 'green' : odf.phase === 'Error' ? 'red' : 'orange';
        document.getElementById('odf-body').innerHTML = `
            <div class="pf-v5-l-flex pf-m-column pf-m-gap-sm">
                <div class="pf-v5-l-flex pf-m-justify-content-space-between">
                    <span class="pf-v5-u-font-weight-bold">${escapeHtml(odf.name)}</span>
                    <span class="pf-v5-c-label pf-m-${color}"><span class="pf-v5-c-label__content">${escapeHtml(odf.phase)}</span></span>
                </div>
            </div>`;
    }

    // --- Node Utilization ---
    async function loadNodeUtilization() {
        try {
            const resp = await fetch('/api/status/nodes');
            const nodes = await resp.json();
            if (!resp.ok) throw new Error(nodes.error || 'request failed');
            renderNodeUtilization(nodes || []);
        } catch (e) {
            document.getElementById('node-util-body').innerHTML = errorMsg(e.message);
        }
    }

    function utilBar(usagePct, requestPct, label) {
        const usageColor = usagePct > 90 ? '#c9190b' : usagePct > 70 ? '#f0ab00' : '#3e8635';
        const reqColor = '#06c';
        const cappedUsage = Math.min(usagePct, 100);
        const cappedReq = Math.min(requestPct, 100);
        return `<div style="position:relative;height:18px;background:#f0f0f0;border-radius:3px;overflow:visible;margin:2px 0;">
            <div style="position:absolute;height:100%;width:${cappedReq}%;background:${reqColor};opacity:0.2;border-radius:3px;" title="Requests: ${requestPct.toFixed(0)}%"></div>
            <div style="position:absolute;height:100%;width:${cappedUsage}%;background:${usageColor};border-radius:3px;" title="Usage: ${usagePct.toFixed(0)}%"></div>
            <span style="position:absolute;right:4px;top:1px;font-size:11px;color:#333;font-weight:600;">${label}</span>
        </div>`;
    }

    function formatMem(bytes) {
        const gb = bytes / 1e9;
        if (gb >= 1) return gb.toFixed(1) + ' GB';
        return (bytes / 1e6).toFixed(0) + ' MB';
    }

    function formatCPU(millicores) {
        return (millicores / 1000).toFixed(2) + ' cores';
    }

    function renderNodeUtilization(nodes) {
        if (nodes.length === 0) {
            document.getElementById('node-util-body').innerHTML = '<span class="pf-v5-u-color-200">No node data</span>';
            return;
        }

        let html = `<table class="pf-v5-c-table pf-m-compact" style="width:100%;">
            <thead><tr>
                <th>Node</th>
                <th>Status</th>
                <th style="width:22%;">CPU Usage / Requests</th>
                <th style="width:22%;">Memory Usage / Requests</th>
                <th>Pods</th>
            </tr></thead><tbody>`;

        for (const n of nodes) {
            const statusColor = n.status === 'Ready' ? 'green' : 'red';

            const cpuLabel = `${formatCPU(n.cpuUsage)} / ${formatCPU(n.cpuAllocatable)}`;
            const memLabel = `${formatMem(n.memUsage)} / ${formatMem(n.memAllocatable)}`;

            const cpuOvercommit = n.cpuOvercommitPct > 100
                ? `<span class="pf-v5-u-font-size-xs pf-v5-u-danger-color-100" title="CPU overcommitted">${n.cpuOvercommitPct.toFixed(0)}% req</span>`
                : `<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${n.cpuOvercommitPct.toFixed(0)}% req</span>`;

            const memOvercommit = n.memOvercommitPct > 100
                ? `<span class="pf-v5-u-font-size-xs pf-v5-u-danger-color-100" title="Memory overcommitted">${n.memOvercommitPct.toFixed(0)}% req</span>`
                : `<span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${n.memOvercommitPct.toFixed(0)}% req</span>`;

            html += `<tr>
                <td>
                    <div class="pf-v5-u-font-size-sm">${escapeHtml(n.name)}</div>
                </td>
                <td><span class="pf-v5-c-label pf-m-${statusColor}"><span class="pf-v5-c-label__content">${escapeHtml(n.status)}</span></span></td>
                <td>
                    ${utilBar(n.cpuUsagePct, n.cpuOvercommitPct, n.cpuUsagePct.toFixed(0) + '%')}
                    <div class="pf-v5-l-flex pf-m-justify-content-space-between" style="margin-top:2px;">
                        <span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${cpuLabel}</span>
                        ${cpuOvercommit}
                    </div>
                </td>
                <td>
                    ${utilBar(n.memUsagePct, n.memOvercommitPct, n.memUsagePct.toFixed(0) + '%')}
                    <div class="pf-v5-l-flex pf-m-justify-content-space-between" style="margin-top:2px;">
                        <span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${memLabel}</span>
                        ${memOvercommit}
                    </div>
                </td>
                <td>
                    <span class="pf-v5-u-font-size-sm">${n.podCount} / ${n.podCapacity}</span>
                </td>
            </tr>`;
        }

        html += '</tbody></table>';
        html += `<div class="pf-v5-u-font-size-xs pf-v5-u-color-200 pf-v5-u-mt-sm">
            <span style="display:inline-block;width:12px;height:12px;background:#3e8635;border-radius:2px;vertical-align:middle;margin-right:4px;"></span>Usage
            <span style="display:inline-block;width:12px;height:12px;background:#06c;opacity:0.2;border-radius:2px;vertical-align:middle;margin-left:12px;margin-right:4px;"></span>Requests
            <span style="margin-left:12px;">Overcommitted when requests &gt; 100%</span>
        </div>`;

        document.getElementById('node-util-body').innerHTML = html;
    }

    // --- Top Consumers ---
    async function loadTopConsumers() {
        try {
            const resp = await fetch('/api/status/top');
            const data = await resp.json();
            if (!resp.ok) throw new Error(data.error || 'request failed');
            renderTopTable('top-pods-body', data.pods || [], 'Pod');
            renderTopTable('top-vms-body', data.vms || [], 'VM');
        } catch (e) {
            document.getElementById('top-pods-body').innerHTML = errorMsg(e.message);
            document.getElementById('top-vms-body').innerHTML = errorMsg(e.message);
        }
    }

    function renderTopTable(elementId, items, typeLabel) {
        if (items.length === 0) {
            document.getElementById(elementId).innerHTML = `<span class="pf-v5-u-color-200">No ${typeLabel.toLowerCase()}s found</span>`;
            return;
        }

        let html = `<table class="pf-v5-c-table pf-m-compact" style="width:100%;">
            <thead><tr>
                <th>${typeLabel}</th>
                <th>Namespace</th>
                <th>CPU Usage</th>
                <th>CPU Req</th>
                <th>Mem Usage</th>
                <th>Mem Req</th>
            </tr></thead><tbody>`;

        for (const item of items) {
            html += `<tr>
                <td class="pf-v5-u-font-size-sm" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</td>
                <td class="pf-v5-u-font-size-sm pf-v5-u-color-200">${escapeHtml(item.namespace)}</td>
                <td class="pf-v5-u-font-size-sm">${item.cpuStr}</td>
                <td class="pf-v5-u-font-size-sm pf-v5-u-color-200">${item.cpuReqStr || '-'}</td>
                <td class="pf-v5-u-font-size-sm">${item.memStr}</td>
                <td class="pf-v5-u-font-size-sm pf-v5-u-color-200">${item.memReqStr || '-'}</td>
            </tr>`;
        }

        html += '</tbody></table>';
        document.getElementById(elementId).innerHTML = html;
    }

    // --- Etcd Health ---
    async function loadEtcdHealth() {
        try {
            const resp = await fetch('/api/status/etcd');
            if (resp.status === 404) {
                document.getElementById('etcd-card').classList.add('hidden');
                return;
            }
            const data = await resp.json();
            if (!resp.ok) throw new Error(data.error || 'request failed');
            renderEtcd(data);
        } catch (e) {
            document.getElementById('etcd-body').innerHTML = errorMsg(e.message);
        }
    }

    function renderEtcd(data) {
        if (!data.members || data.members.length === 0) {
            document.getElementById('etcd-body').innerHTML = '<span class="pf-v5-u-color-200">No etcd data available</span>';
            return;
        }

        const healthColor = data.healthy ? 'green' : 'red';

        let html = `<div class="pf-v5-l-flex pf-m-column pf-m-gap-sm">`;
        html += `<div class="pf-v5-l-flex pf-m-justify-content-space-between">
            <span class="pf-v5-u-font-weight-bold">Health</span>
            <span class="pf-v5-c-label pf-m-${healthColor}"><span class="pf-v5-c-label__content">${data.healthy ? 'Healthy' : 'Unhealthy'}</span></span>
        </div>`;
        html += `<hr class="pf-v5-c-divider" style="margin:4px 0;">`;

        for (const m of data.members) {
            const leaderBadge = m.isLeader ? ' <span class="pf-v5-c-label pf-m-blue pf-m-compact"><span class="pf-v5-c-label__content">Leader</span></span>' : '';
            html += `<div class="pf-v5-l-flex pf-m-justify-content-space-between pf-m-align-items-center">
                <div>
                    <span class="pf-v5-u-font-size-sm">${escapeHtml(m.pod)}</span>${leaderBadge}
                </div>
            </div>`;
            html += `<div class="pf-v5-l-flex pf-m-gap-lg pf-v5-u-font-size-xs pf-v5-u-color-200" style="margin-top:-4px;">
                <span>Rev: ${m.revision.toLocaleString()}</span>
                <span>DB: ${m.dbSizeMB.toFixed(1)} MB</span>
            </div>`;
        }

        html += '</div>';
        document.getElementById('etcd-body').innerHTML = html;
    }

    // --- GPU Utilization ---
    async function loadGPUs() {
        try {
            const resp = await fetch('/api/status/gpus');
            const gpus = await resp.json();
            if (!resp.ok) throw new Error(gpus.error || 'request failed');
            if (!gpus || gpus.length === 0) return;
            document.getElementById('gpu-section').classList.remove('hidden');
            renderGPUs(gpus);
        } catch (e) {
            // No GPU nodes, keep hidden
        }
    }

    function renderGPUs(nodes) {
        let html = `<table class="pf-v5-c-table pf-m-compact" style="width:100%;">
            <thead><tr>
                <th>Node</th>
                <th>Type</th>
                <th>Status</th>
                <th style="width:22%;">GPU Used / Capacity</th>
                <th>Consumers</th>
            </tr></thead><tbody>`;

        for (const n of nodes) {
            const statusColor = n.status === 'Ready' ? 'green' : 'red';
            const usageColor = n.gpuUsagePct > 90 ? '#c9190b' : n.gpuUsagePct > 70 ? '#f0ab00' : '#3e8635';
            const cappedUsage = Math.min(n.gpuUsagePct, 100);
            const gpuLabel = `${n.gpuUsed} / ${n.gpuCapacity}`;

            let consumersHtml = '-';
            if (n.gpuConsumers && n.gpuConsumers.length > 0) {
                consumersHtml = n.gpuConsumers.map(c =>
                    `<div class="pf-v5-u-font-size-xs"><span class="pf-v5-u-font-weight-bold">${escapeHtml(c.name)}</span> <span class="pf-v5-u-color-200">${escapeHtml(c.namespace)}</span> <span class="pf-v5-c-label pf-m-compact pf-m-blue"><span class="pf-v5-c-label__content">${c.gpus} GPU${c.gpus > 1 ? 's' : ''}</span></span></div>`
                ).join('');
            }

            html += `<tr>
                <td>
                    <div class="pf-v5-u-font-size-sm">${escapeHtml(n.name)}</div>
                </td>
                <td><span class="pf-v5-u-font-size-sm">${escapeHtml(n.gpuType || 'Unknown')}</span></td>
                <td><span class="pf-v5-c-label pf-m-${statusColor}"><span class="pf-v5-c-label__content">${escapeHtml(n.status)}</span></span></td>
                <td>
                    <div style="position:relative;height:18px;background:#f0f0f0;border-radius:3px;overflow:visible;margin:2px 0;">
                        <div style="position:absolute;height:100%;width:${cappedUsage}%;background:${usageColor};border-radius:3px;" title="Used: ${n.gpuUsagePct.toFixed(0)}%"></div>
                        <span style="position:absolute;right:4px;top:1px;font-size:11px;color:#333;font-weight:600;">${gpuLabel}</span>
                    </div>
                    <div class="pf-v5-l-flex pf-m-justify-content-space-between" style="margin-top:2px;">
                        <span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${n.gpuFree} free</span>
                        <span class="pf-v5-u-font-size-xs pf-v5-u-color-200">${n.gpuUsagePct.toFixed(0)}% used</span>
                    </div>
                </td>
                <td>${consumersHtml}</td>
            </tr>`;
        }

        html += '</tbody></table>';

        // Summary row
        const totalCap = nodes.reduce((s, n) => s + n.gpuCapacity, 0);
        const totalUsed = nodes.reduce((s, n) => s + n.gpuUsed, 0);
        const totalFree = totalCap - totalUsed;
        html += `<div class="pf-v5-u-font-size-xs pf-v5-u-color-200 pf-v5-u-mt-sm">
            Total: <span class="pf-v5-u-font-weight-bold">${totalUsed}</span> used / <span class="pf-v5-u-font-weight-bold">${totalCap}</span> capacity (<span class="pf-v5-u-font-weight-bold">${totalFree}</span> free across ${nodes.length} node${nodes.length > 1 ? 's' : ''})
        </div>`;

        document.getElementById('gpu-body').innerHTML = html;
    }

    // --- Helpers ---
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    function errorMsg(msg) {
        return `<div class="pf-v5-u-danger-color-100">${escapeHtml(msg)}</div>`;
    }

    // --- NMState Networks ---
    async function loadNetworks() {
        try {
            const resp = await fetch('/api/status/networks');
            if (!resp.ok) return; // NMState not installed, keep section hidden
            const networks = await resp.json();
            document.getElementById('networks-section').classList.remove('hidden');
            renderNetworks(networks);
        } catch (e) {
            // NMState not available, keep hidden
        }
    }

    function renderNetworks(networks) {
        if (!networks || networks.length === 0) {
            document.getElementById('networks-body').innerHTML = '<span class="pf-v5-u-color-200">No VLANs configured</span>';
            return;
        }

        let html = `<table class="pf-v5-c-table pf-m-compact" style="width:100%;">
            <thead><tr>
                <th>Interface</th>
                <th>Type</th>
                <th>State</th>
                <th>Issues</th>
            </tr></thead><tbody>`;

        for (const net of networks) {
            const stateColor = net.state === 'up' ? 'green' : net.state === 'down' ? 'red' : 'orange';
            const stateLabel = net.state === 'up' ? 'Up (all nodes)' : net.state === 'partial' ? 'Partial' : 'Down';

            let issueHtml = '-';
            if (net.missing && net.missing.length > 0) {
                const detailId = 'missing-' + escapeHtml(net.name).replace(/[^a-zA-Z0-9]/g, '-');
                issueHtml = `<span class="pf-v5-u-danger-color-100" style="cursor:pointer;" onclick="document.getElementById('${detailId}').classList.toggle('hidden')">Missing on ${net.missing.length} node${net.missing.length > 1 ? 's' : ''}</span>
                    <div id="${detailId}" class="hidden pf-v5-u-font-size-xs pf-v5-u-mt-xs pf-v5-u-danger-color-100">${net.missing.map(n => escapeHtml(n)).join(', ')}</div>`;
            }

            html += `<tr>
                <td class="pf-v5-u-font-size-sm pf-v5-u-font-weight-bold">${escapeHtml(net.name)}</td>
                <td class="pf-v5-u-font-size-sm pf-v5-u-color-200">${escapeHtml(net.type)}</td>
                <td><span class="pf-v5-c-label pf-m-${stateColor} pf-m-compact"><span class="pf-v5-c-label__content">${stateLabel}</span></span></td>
                <td>${issueHtml}</td>
            </tr>`;
        }

        html += '</tbody></table>';
        document.getElementById('networks-body').innerHTML = html;
    }

    // --- Storage Classes ---
    async function loadStorageClasses() {
        try {
            const resp = await fetch('/api/status/storageclasses');
            const scs = await resp.json();
            if (!resp.ok) throw new Error(scs.error || 'request failed');
            renderStorageClasses(scs || []);
        } catch (e) {
            document.getElementById('storageclasses-body').innerHTML = errorMsg(e.message);
        }
    }

    function renderStorageClasses(scs) {
        if (scs.length === 0) {
            document.getElementById('storageclasses-body').innerHTML = '<span class="pf-v5-u-color-200">No storage classes found</span>';
            return;
        }

        let html = `<table class="pf-v5-c-table pf-m-compact" style="width:100%;">
            <thead><tr>
                <th>Name</th>
                <th>Provisioner</th>
                <th>Reclaim Policy</th>
                <th>Binding Mode</th>
            </tr></thead><tbody>`;

        for (const sc of scs) {
            const defaultBadge = sc.isDefault ? ' <span class="pf-v5-c-label pf-m-blue pf-m-compact"><span class="pf-v5-c-label__content">Default</span></span>' : '';
            html += `<tr>
                <td class="pf-v5-u-font-size-sm"><span class="pf-v5-u-font-weight-bold">${escapeHtml(sc.name)}</span>${defaultBadge}</td>
                <td class="pf-v5-u-font-size-sm pf-v5-u-color-200">${escapeHtml(sc.provisioner)}</td>
                <td class="pf-v5-u-font-size-sm">${escapeHtml(sc.reclaimPolicy)}</td>
                <td class="pf-v5-u-font-size-sm">${escapeHtml(sc.volumeBindingMode)}</td>
            </tr>`;
        }

        html += '</tbody></table>';
        document.getElementById('storageclasses-body').innerHTML = html;
    }

    // --- Capabilities ---
    async function loadCapabilities() {
        try {
            const resp = await fetch('/api/support/capabilities');
            if (!resp.ok) return;
            const caps = await resp.json();
            if (caps.cnv) {
                const vmsCard = document.getElementById('top-vms-card');
                if (vmsCard) vmsCard.style.display = '';
            }
        } catch (e) {
            // ignore
        }
    }

    // --- Init ---
    loadClusterHealth();
    loadNodeUtilization();
    loadGPUs();
    loadTopConsumers();
    loadEtcdHealth();
    loadNetworks();
    loadStorageClasses();
    loadCapabilities();

    setInterval(loadClusterHealth, 60000);
    setInterval(loadNodeUtilization, 60000);
    setInterval(loadGPUs, 60000);
    setInterval(loadTopConsumers, 60000);
    setInterval(loadEtcdHealth, 60000);
    setInterval(loadNetworks, 60000);
    setInterval(loadStorageClasses, 60000);
})();
