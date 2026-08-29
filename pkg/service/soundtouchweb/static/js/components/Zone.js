import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { resolvedZoneMember } from '../devicePresentation.js';
import {
    effectiveZoneDetail,
    physicalMemberMetadata,
    zoneMemberCountSummary,
    zoneMemberIdentifiers,
    zoneMemberMetadata,
    zoneTopologyFingerprint,
} from '../zonePresentation.mjs';

const html = htm.bind(h);

function currentSourceAllowsMultiroom(device) {
    const nowPlaying = device?.status?.nowPlaying;
    const source = nowPlaying?.Source;
    if (!source || source === 'STANDBY' || source === 'INVALID_SOURCE') return false;

    return (device?.status?.sources?.SourceItem || []).some(item =>
        item.Source === source && item.MultiroomAllowed &&
        (!nowPlaying.SourceAccount || item.SourceAccount === nowPlaying.SourceAccount));
}

function deduplicateMembers(members) {
    const seen = new Set();

    return members.filter(({ controlId, member }) => {
        const key = controlId || member?.deviceIds?.join('|');
        if (!key || seen.has(key)) return false;
        seen.add(key);
        return true;
    });
}

function inferredPhysicalCount(members) {
    return members.reduce((total, { member }) =>
        total + Math.max(1, member?.physicalMembers?.length || 0), 0);
}

export function Zone({ deviceId, devices }) {
    const [zone, setZone] = useState(null);
    const [candidates, setCandidates] = useState({});
    const [loading, setLoading] = useState(true);
    const [showPicker, setShowPicker] = useState(false);
    const canGroup = currentSourceAllowsMultiroom(devices?.[deviceId]);
    const projection = devices?.[zone?.masterIp || deviceId]?.zone || devices?.[deviceId]?.zone;
    const topologyFingerprint = zoneTopologyFingerprint(projection);
    const observedTopology = useRef({ deviceId, fingerprint: topologyFingerprint });
    const refreshGeneration = useRef(0);

    function refresh() {
        const generation = ++refreshGeneration.current;
        Promise.all([api.zone(deviceId), api.zoneCandidates(deviceId)])
            .then(([zoneResp, candidatesResp]) => {
                if (generation !== refreshGeneration.current) return;
                if (zoneResp.success) setZone(zoneResp.data);
                if (candidatesResp.success) setCandidates(candidatesResp.data || {});
            })
            .finally(() => {
                if (generation === refreshGeneration.current) setLoading(false);
            });
    }

    useEffect(() => { refresh(); }, [deviceId]);
    useEffect(() => {
        const previous = observedTopology.current;
        observedTopology.current = { deviceId, fingerprint: topologyFingerprint };
        if (previous.deviceId !== deviceId || previous.fingerprint === topologyFingerprint) return;

        setLoading(true);
        refresh();
    }, [deviceId, topologyFingerprint]);
    useEffect(() => {
        if (!canGroup) setShowPicker(false);
    }, [canGroup]);

    async function addDevice(slaveId) {
        if (!canGroup) return;
        setShowPicker(false);
        await api.zoneAdd(deviceId, slaveId);
        refresh();
    }

    async function removeDevice(slaveId) {
        await api.zoneRemove(deviceId, slaveId);
        refresh();
    }

    async function dissolve() {
        await api.zoneDissolve(deviceId);
        refresh();
    }

    async function leave() {
        await api.zoneLeave(deviceId);
        refresh();
    }

    if (loading) return html`
        <div class="zone-section">
            <div class="section-title">Zone</div>
            <div class="loading-bar"></div>
        </div>
    `;

    const currentZone = effectiveZoneDetail(zone, projection, devices?.[deviceId], deviceId);
    if (!currentZone) return null;

    const projectedMaster = (projection?.members || []).find(member =>
        member.controlId === projection?.masterControlId ||
        member.deviceIds?.includes(projection?.masterDeviceId));
    const master = currentZone.master || projectedMaster || {
        controlId: currentZone.masterIp || deviceId,
        ip: currentZone.masterIp || deviceId,
        name: currentZone.masterName,
        deviceIds: currentZone.masterHwId ? [currentZone.masterHwId] : [],
    };
    const resolvedMaster = resolvedZoneMember(projection, master);
    const resolvedMembers = (currentZone.members || []).map(member => resolvedZoneMember(projection, member));
    const logicalMembers = deduplicateMembers([
        ...(resolvedMaster.controlId ? [resolvedMaster] : []),
        ...resolvedMembers,
    ]);
    const physicalCount = Number.isInteger(currentZone.physicalMemberCount)
        ? currentZone.physicalMemberCount
        : (Number.isInteger(projection?.physicalMemberCount)
            ? projection.physicalMemberCount
            : inferredPhysicalCount(logicalMembers));

    // Source candidates independently from Group projection while recognizing
    // the logical IDs already represented by the current zone.
    const zoneIds = zoneMemberIdentifiers(
        { controlId: currentZone.masterIp, deviceId: currentZone.masterHwId },
        master,
        currentZone.members,
        projection?.members,
    );
    const available = Object.entries(candidates).filter(([id]) => !zoneIds.has(id));

    const deviceName = (ip) => devices[ip]?.info?.name || candidates[ip]?.info?.name || ip;

    function logicalMemberRow(resolved, isMaster) {
        const member = resolved.member;
        const metadata = zoneMemberMetadata(member);
        const isStereoPair = member?.kind === 'stereoPair';

        return html`
            <div class="zone-logical-member" key=${resolved.controlId}>
                <div class="zone-logical-header">
                    <span class="device-indicator ${metadata.connectivity}" role="status"
                          title=${metadata.connectivityLabel}
                          aria-label=${metadata.statusAriaLabel}></span>
                    <div class="zone-logical-identity">
                        <div class="zone-logical-name">
                            ${metadata.name}
                            ${isMaster ? html`<span class="zone-badge master">Master</span>` : null}
                        </div>
                        <div class="zone-logical-metadata">
                            <span>${metadata.modelType}</span>
                            ${metadata.ip ? html`<span class="zone-logical-ip">${metadata.ip}</span>` : null}
                            <span>${metadata.kind}</span>
                        </div>
                    </div>
                </div>

                ${isStereoPair ? html`
                    <div class="zone-physical-members">
                        ${(member.physicalMembers || []).map(physical => {
                            const diagnostic = physicalMemberMetadata(physical);
                            return html`
                                <div class="zone-physical-member" key=${physical.deviceId || diagnostic.role}>
                                    <span class="zone-physical-role">${diagnostic.role}</span>
                                    <span class="device-indicator ${diagnostic.connectivity}" role="status"
                                          title=${diagnostic.connectivityLabel}
                                          aria-label=${diagnostic.statusAriaLabel}></span>
                                    <div class="zone-physical-identity">
                                        <span class="zone-physical-name">${diagnostic.name}</span>
                                        <span class="zone-physical-metadata">
                                            <span>${diagnostic.modelType}</span>
                                            ${diagnostic.ip ? html`
                                                <span class="zone-logical-ip">${diagnostic.ip}</span>
                                            ` : null}
                                        </span>
                                    </div>
                                </div>
                            `;
                        })}
                    </div>
                ` : null}
            </div>
        `;
    }

    return html`
        <div class="zone-section">
            <div class="section-title">Zone</div>

            ${currentZone.isStandalone && html`
                <div class="zone-row">
                    <span class="zone-status-label">Standalone</span>
                    ${available.length > 0 && html`
                        <button class="btn-secondary zone-btn" onClick=${() => setShowPicker(true)}
                            disabled=${!canGroup}>+ Group with…</button>
                    `}
                </div>
                ${available.length > 0 && !canGroup && html`
                    <div class="zone-status-label">Start a multiroom-capable source before grouping speakers.</div>
                `}
            `}

            ${!currentZone.isStandalone && logicalMembers.length > 0 ? html`
                <details class="zone-member-details">
                    <summary>${zoneMemberCountSummary(logicalMembers.length, physicalCount)}</summary>
                    <div class="zone-logical-members">
                        ${logicalMembers.map(member => logicalMemberRow(
                            member,
                            member.controlId === resolvedMaster.controlId,
                        ))}
                    </div>
                </details>
            ` : null}

            ${currentZone.isMaster && html`
                <div class="zone-management">
                    <div class="zone-management-title">Zone management</div>
                    ${resolvedMembers.length > 0 ? html`
                        <div class="zone-management-members">
                            ${resolvedMembers.map(resolved => {
                                const memberID = resolved.member?.ip || resolved.controlId;
                                const name = resolved.name || deviceName(memberID);
                                return html`
                                    <div class="zone-management-member" key=${resolved.controlId}>
                                        <span>${name}</span>
                                        <button class="btn-icon zone-remove" title="Remove from zone"
                                            aria-label=${`Remove ${name} from zone`}
                                            onClick=${() => removeDevice(memberID)}>✕</button>
                                    </div>
                                `;
                            })}
                        </div>
                    ` : null}
                    <div class="zone-actions">
                        ${available.length > 0 && html`
                            <button class="btn-secondary zone-btn" onClick=${() => setShowPicker(true)}
                                disabled=${!canGroup}>+ Add speaker</button>
                        `}
                        <button class="btn-secondary zone-btn" onClick=${dissolve}>Dissolve zone</button>
                    </div>
                </div>
            `}

            ${currentZone.isSlave && html`
                <div class="zone-management">
                    <div class="zone-management-title">Zone management</div>
                    <div class="zone-row">
                        <span class="zone-status-label">Zone: ${currentZone.masterName || deviceName(currentZone.masterIp)}</span>
                        <button class="btn-secondary zone-btn" onClick=${leave}>Leave zone</button>
                    </div>
                </div>
            `}

            ${showPicker && canGroup && html`
                <div class="overlay" onClick=${() => setShowPicker(false)}>
                    <div class="device-picker" onClick=${e => e.stopPropagation()}>
                        <div class="picker-title">Add to zone</div>
                        <div class="picker-devices">
                            ${available.map(([ip, d]) => html`
                                <button class="picker-device-btn" key=${ip} onClick=${() => addDevice(ip)}>
                                    <div class="picker-device-info">
                                        <span class="picker-device-name">${d.info?.name || ip}</span>
                                        <span class="picker-device-ip">${d.info?.ip_address || ip}</span>
                                    </div>
                                </button>
                            `)}
                        </div>
                        <button class="btn-secondary picker-cancel" onClick=${() => setShowPicker(false)}>Cancel</button>
                    </div>
                </div>
            `}
        </div>
    `;
}
