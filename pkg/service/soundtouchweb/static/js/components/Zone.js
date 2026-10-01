import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { resolvedZoneMember } from '../devicePresentation.js';
import {
    effectiveZoneDetail,
    isCurrentZoneRefresh,
    zoneRefreshContext,
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

export function Zone({ deviceId, devices, onSelectMember }) {
    const [zone, setZone] = useState(null);
    const [candidates, setCandidates] = useState({});
    const [loading, setLoading] = useState(true);
    const [showPicker, setShowPicker] = useState(false);
    const [mutationError, setMutationError] = useState('');
    const canGroup = currentSourceAllowsMultiroom(devices?.[deviceId]);
    const projection = devices?.[zone?.masterIp || deviceId]?.zone || devices?.[deviceId]?.zone;
    const topologyFingerprint = zoneTopologyFingerprint(projection);
    const refreshContext = useRef(null);
    refreshContext.current = zoneRefreshContext(refreshContext.current, deviceId, topologyFingerprint);
    const mutationContext = useRef(null);
    if (mutationContext.current?.deviceId !== deviceId) {
        mutationContext.current = { deviceId, generation: 0 };
    }

    function refresh() {
        const context = refreshContext.current;
        const generation = ++context.generation;
        Promise.all([api.zone(deviceId), api.zoneCandidates(deviceId)])
            .then(([zoneResp, candidatesResp]) => {
                if (!isCurrentZoneRefresh(context, refreshContext.current, generation)) return;
                if (zoneResp.success) setZone(zoneResp.data);
                if (candidatesResp.success) setCandidates(candidatesResp.data || {});
            })
            .finally(() => {
                if (isCurrentZoneRefresh(context, refreshContext.current, generation)) setLoading(false);
            });
    }

    useEffect(() => { refresh(); }, [deviceId, topologyFingerprint]);
    useEffect(() => () => {
        refreshContext.current = null;
        mutationContext.current = null;
    }, []);
    useEffect(() => { setMutationError(''); }, [deviceId]);
    useEffect(() => {
        if (!canGroup) setShowPicker(false);
    }, [canGroup]);

    async function mutate(action) {
        // A mutation's own broadcast must not hide its eventual readback error.
        const context = mutationContext.current;
        const generation = ++context.generation;
        const isCurrent = () => context === mutationContext.current && generation === context.generation;
        setMutationError('');
        try {
            const response = await action();
            if (!isCurrent()) return;
            if (!response?.success) {
                setMutationError(response?.error || 'Zone operation failed');
                return;
            }
            refresh();
        } catch (error) {
            if (isCurrent()) {
                setMutationError(error?.message || 'Zone operation failed');
            }
        }
    }

    async function addDevice(slaveId) {
        if (!canGroup) return;
        setShowPicker(false);
        await mutate(() => api.zoneAdd(deviceId, slaveId));
    }

    async function removeDevice(slaveId) {
        await mutate(() => api.zoneRemove(deviceId, slaveId));
    }

    async function dissolve() {
        await mutate(() => api.zoneDissolve(deviceId));
    }

    async function leave() {
        await mutate(() => api.zoneLeave(deviceId));
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
        // Only a row that leads somewhere is a button: the member must be in
        // the inventory, and the page already open is not a destination.
        const openable = Boolean(onSelectMember) && resolved.controlId !== deviceId &&
            Boolean(devices?.[resolved.controlId]);
        const identity = html`
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
        `;

        return html`
            <div class="zone-logical-member" key=${resolved.controlId}>
                ${openable ? html`
                    <button class="zone-logical-header zone-member-open" type="button"
                            aria-label=${`Open details for ${metadata.name}`}
                            onClick=${() => onSelectMember(resolved.controlId)}>
                        ${identity}
                        <svg class="zone-member-open-icon" width="18" height="18" viewBox="0 0 24 24"
                             fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"
                             stroke-linejoin="round" aria-hidden="true">
                            <path d="m9 18 6-6-6-6" />
                        </svg>
                    </button>
                ` : html`
                    <div class="zone-logical-header">
                        ${identity}
                    </div>
                `}

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
            ${mutationError ? html`<div class="settings-error" role="alert">${mutationError}</div>` : null}

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
