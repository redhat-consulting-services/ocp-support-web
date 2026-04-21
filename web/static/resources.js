(function() {
    'use strict';

    var lastResults = [];
    var currentNS = '';

    async function loadCapabilities() {
        try {
            var res = await fetch('/api/support/capabilities');
            var caps = await res.json();
            if (caps.acm) document.getElementById('acm-nav').style.display = '';
        } catch(e) { /* ignore */ }
    }

    async function loadNamespaces() {
        try {
            var res = await fetch('/api/support/namespaces');
            var ns = await res.json();
            var sel = document.getElementById('ns-select');
            ns.forEach(function(n) {
                var opt = document.createElement('option');
                opt.value = n;
                opt.textContent = n;
                sel.appendChild(opt);
            });
        } catch(e) {
            console.error('Failed to load namespaces', e);
        }
    }

    async function scanNamespace() {
        var ns = document.getElementById('ns-select').value;
        currentNS = ns;
        var container = document.getElementById('resource-groups');

        if (!ns) {
            lastResults = [];
            container.innerHTML =
                '<p class="pf-v5-u-color-200 pf-v5-u-text-align-center pf-v5-u-py-xl">' +
                'Select a namespace to browse resources.</p>';
            return;
        }

        container.innerHTML =
            '<div class="pf-v5-u-text-align-center pf-v5-u-py-xl">' +
            '<span class="pf-v5-c-spinner pf-m-lg" role="progressbar">' +
            '<span class="pf-v5-c-spinner__clipper"></span>' +
            '<span class="pf-v5-c-spinner__lead-ball"></span>' +
            '<span class="pf-v5-c-spinner__tail-ball"></span></span>' +
            '<p class="pf-v5-u-mt-sm">Scanning resources in ' + escapeHtml(ns) + '...</p></div>';

        try {
            var res = await fetch('/api/resources/ns?ns=' + encodeURIComponent(ns));
            lastResults = await res.json();
            renderResults();
        } catch(e) {
            container.innerHTML =
                '<div class="pf-v5-c-alert pf-m-danger">' +
                '<div class="pf-v5-c-alert__title">Failed to scan namespace.</div></div>';
        }
    }

    function renderResults() {
        var filter = document.getElementById('resource-filter').value.toLowerCase();
        var container = document.getElementById('resource-groups');
        container.innerHTML = '';

        if (lastResults.length === 0) {
            container.innerHTML = '<p class="pf-v5-u-color-200">No resources found in ' +
                escapeHtml(currentNS) + '.</p>';
            return;
        }

        var groups = {};
        lastResults.forEach(function(r) {
            var key = r.group || '';
            if (!groups[key]) groups[key] = [];
            groups[key].push(r);
        });

        var hasResults = false;
        var groupKeys = Object.keys(groups).sort(function(a, b) {
            if (a === '') return -1;
            if (b === '') return 1;
            return a.localeCompare(b);
        });

        groupKeys.forEach(function(key) {
            var resources = groups[key];

            if (filter) {
                resources = resources.filter(function(r) {
                    return r.resource.indexOf(filter) !== -1 ||
                           r.kind.toLowerCase().indexOf(filter) !== -1 ||
                           (r.group && r.group.indexOf(filter) !== -1);
                });
            }
            if (resources.length === 0) return;
            hasResults = true;

            var section = document.createElement('div');
            section.className = 'pf-v5-u-mb-lg';

            var title = document.createElement('div');
            title.className = 'resource-group-title';
            title.textContent = (key || 'core') + ' (' + resources[0].version + ')';
            section.appendChild(title);

            resources.forEach(function(r) {
                var card = document.createElement('div');
                card.className = 'pf-v5-c-card pf-v5-u-mb-sm status-card';

                var cardTitle = document.createElement('div');
                cardTitle.className = 'pf-v5-c-card__title';
                cardTitle.style.cursor = 'pointer';
                cardTitle.innerHTML =
                    '<div class="pf-v5-l-flex pf-m-align-items-center pf-m-justify-content-space-between">' +
                    '<h3 class="pf-v5-c-card__title-text" style="margin:0;">' +
                    escapeHtml(r.kind) +
                    ' <span class="pf-v5-u-font-weight-normal pf-v5-u-color-200">(' + r.items.length + ')</span>' +
                    '</h3>' +
                    '<span class="pf-v5-u-color-200 pf-v5-u-font-size-sm">' + escapeHtml(r.resource) + '</span>' +
                    '</div>';

                var cardBody = document.createElement('div');
                cardBody.className = 'pf-v5-c-card__body';

                var html = '<table class="pf-v5-c-table pf-m-compact pf-m-grid-md">';
                html += '<thead><tr><th>Name</th><th>Age</th><th></th></tr></thead>';
                html += '<tbody>';
                r.items.forEach(function(item) {
                    html += '<tr>';
                    html += '<td><strong>' + escapeHtml(item.name) + '</strong></td>';
                    html += '<td>' + formatAge(item.created) + '</td>';
                    html += '<td><button class="pf-v5-c-button pf-m-link pf-m-small view-yaml-btn" ' +
                        'data-group="' + escapeAttr(r.group) + '" ' +
                        'data-version="' + escapeAttr(r.version) + '" ' +
                        'data-resource="' + escapeAttr(r.resource) + '" ' +
                        'data-name="' + escapeAttr(item.name) + '">View YAML</button></td>';
                    html += '</tr>';
                });
                html += '</tbody></table>';
                cardBody.innerHTML = html;

                cardTitle.addEventListener('click', function() {
                    cardBody.style.display = cardBody.style.display === 'none' ? '' : 'none';
                });

                card.appendChild(cardTitle);
                card.appendChild(cardBody);
                section.appendChild(card);
            });

            container.appendChild(section);
        });

        if (!hasResults) {
            container.innerHTML = '<p class="pf-v5-u-color-200">No matching resources found.</p>';
            return;
        }

        container.querySelectorAll('.view-yaml-btn').forEach(function(btn) {
            btn.addEventListener('click', function(e) {
                e.stopPropagation();
                viewResource(btn.dataset.group, btn.dataset.version,
                             btn.dataset.resource, btn.dataset.name);
            });
        });
    }

    async function viewResource(group, version, resource, name) {
        var modal = document.getElementById('yaml-modal');
        modal.style.display = '';
        document.getElementById('yaml-modal-title').textContent = resource + '/' + name;
        document.getElementById('yaml-modal-content').textContent = 'Loading...';

        try {
            var params = new URLSearchParams({
                ns: currentNS, group: group, version: version,
                resource: resource, name: name
            });
            var res = await fetch('/api/resources/get?' + params);
            var data = await res.text();
            document.getElementById('yaml-modal-content').textContent = data;
        } catch(e) {
            document.getElementById('yaml-modal-content').textContent = 'Failed to load resource.';
        }
    }

    function formatAge(timestamp) {
        if (!timestamp) return '';
        var d = new Date(timestamp);
        var now = new Date();
        var diff = Math.floor((now - d) / 1000);
        if (diff < 0) return '0s';
        if (diff < 60) return diff + 's';
        if (diff < 3600) return Math.floor(diff / 60) + 'm';
        if (diff < 86400) return Math.floor(diff / 3600) + 'h';
        return Math.floor(diff / 86400) + 'd';
    }

    function escapeHtml(s) {
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(s));
        return div.innerHTML;
    }

    function escapeAttr(s) {
        return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    document.getElementById('ns-select').addEventListener('change', scanNamespace);
    document.getElementById('resource-filter').addEventListener('input', function() {
        if (lastResults.length > 0) renderResults();
    });

    document.getElementById('yaml-modal-close').addEventListener('click', function() {
        document.getElementById('yaml-modal').style.display = 'none';
    });
    document.getElementById('yaml-close-btn').addEventListener('click', function() {
        document.getElementById('yaml-modal').style.display = 'none';
    });
    document.getElementById('yaml-copy-btn').addEventListener('click', function() {
        var content = document.getElementById('yaml-modal-content').textContent;
        navigator.clipboard.writeText(content);
        this.textContent = 'Copied!';
        var btn = this;
        setTimeout(function() { btn.textContent = 'Copy'; }, 2000);
    });

    loadCapabilities();
    loadNamespaces();

    document.getElementById('resource-groups').innerHTML =
        '<p class="pf-v5-u-color-200 pf-v5-u-text-align-center pf-v5-u-py-xl">' +
        'Select a namespace to browse resources.</p>';
})();
