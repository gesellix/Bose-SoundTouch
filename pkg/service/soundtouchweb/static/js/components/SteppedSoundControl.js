import { h } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { createSteppedInteraction } from '../steppedInteraction.mjs';
import { balanceLabel, writeSoundSetting } from '../soundSettingsPresentation.mjs';

const html = htm.bind(h);

export function SteppedSoundControl({ kind, target, control, ownerKey, available = true }) {
    const [, redraw] = useState(0);
    const interaction = useRef(null);
    if (!interaction.current) interaction.current = createSteppedInteraction({
        send: (current, value) => writeSoundSetting(api, current, value),
        onChange: () => redraw(value => value + 1),
    });
    const status = target.status;
    const disabled = !available || status?.connectivity !== 'online';
    interaction.current.update({
        ...control, ...target, kind,
        key: JSON.stringify([ownerKey, target.controlId, target.physicalId, status?.epoch, target.pairId || '', kind]),
        // Aggregate status revisions include volume and transport updates.
        // Only this acoustic value can recover this control's outcome.
        projection: JSON.stringify([kind, control.value]),
        disabled,
    });
    useEffect(() => () => interaction.current.dispose(), []);
    const { value, failure, busy, confirmedValue } = interaction.current.view();
    const label = kind === 'bass' ? 'Bass reduction' : 'Stereo balance';
    const display = kind === 'balance' ? balanceLabel(value) : String(value);
    const actual = confirmedValue ?? control.value;
    const readback = kind === 'balance' ? balanceLabel(actual) : String(actual);
    const steps = Array.from({ length: control.max - control.min + 1 }, (_, index) => control.min + index);

    return html`
        <div class="sound-control" aria-busy=${busy ? 'true' : 'false'}>
            <div class="sound-control-title">${label} · ${target.name}</div>
            <div class="sound-control-scale" role="slider" tabIndex=${disabled ? -1 : 0}
                aria-label=${`${label} · ${target.name}`} aria-disabled=${disabled ? 'true' : 'false'}
                aria-valuemin=${control.min} aria-valuemax=${control.max} aria-valuenow=${value}
                aria-valuetext=${display}
                onKeyDown=${event => { if (interaction.current.keyDown(event.key)) event.preventDefault(); }}
                onPointerDown=${event => {
                    interaction.current.beginTap(event);
                    if (!disabled && event.button === 0 && event.isPrimary !== false) event.currentTarget.setPointerCapture(event.pointerId);
                }}
                onPointerMove=${event => interaction.current.moveTap(event)}
                onPointerUp=${event => interaction.current.endTap(event, event.currentTarget.getBoundingClientRect())}
                onPointerCancel=${() => interaction.current.cancelTap()}
                onLostPointerCapture=${() => interaction.current.cancelTap()}
                onBlur=${() => interaction.current.cancelTap()}>
                <div class="sound-control-scale-track" aria-hidden="true">
                    ${steps.map(step => html`<span key=${step}
                        class=${`sound-control-tick${step === value ? ' selected' : ''}${step === control.defaultValue ? ' default' : ''}`}>
                        <span class="sound-control-tick-mark"></span>
                        <span class="sound-control-tick-label">${step === control.min || step === control.max || step === control.defaultValue ? step : ''}</span>
                    </span>`)}
                </div>
            </div>
            <div class="sound-control-scale-hint">Tap the scale or use arrow keys to choose a step.</div>
            <div class="sound-control-steps" role="group" aria-label=${`${label} · ${target.name}`}>
                <button type="button" class="btn-secondary" data-action="decrease" disabled=${disabled || value <= control.min}
                    aria-label=${kind === 'balance' ? 'One step left' : `Reduce bass · ${target.name}`}
                    onClick=${() => interaction.current.step(-1)}>${kind === 'balance' ? 'Left' : '−'}</button>
                <output aria-label=${`${label} value`}>${display}</output>
                <button type="button" class="btn-secondary" data-action="increase" disabled=${disabled || value >= control.max}
                    aria-label=${kind === 'balance' ? 'One step right' : `Increase bass · ${target.name}`}
                    onClick=${() => interaction.current.step(1)}>${kind === 'balance' ? 'Right' : '+'}</button>
                <button type="button" class="btn-secondary" data-action="reset" disabled=${disabled || value === control.defaultValue}
                    onClick=${() => interaction.current.choose(control.defaultValue)}>Reset to default</button>
            </div>
            <div class="sound-control-readback">Speaker: ${readback} · Range ${control.min}…${control.max} · Default ${control.defaultValue}</div>
            ${disabled ? html`<div class="sound-control-readback">Currently unavailable</div>` : null}
            ${failure ? html`<div class="settings-error" role="alert">${failure}</div>` : null}
        </div>
    `;
}
