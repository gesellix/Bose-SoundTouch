import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

const SLOTS = [1, 2, 3, 4, 5, 6];

// PresetPicker is the ★ button plus its 1–6 slot popover, shared by the
// now-playing card and the Library rows (issue 700). It owns the popover
// state and the save feedback; what a save *does* is the caller's business.
//
// onSave(slot) may return a thenable, in which case the picker shows the
// saving / ✓ / ✗ feedback inline: both current callers do, because the store
// is a single request whose result is known immediately. A caller that
// returns nothing is saying the outcome will be reported elsewhere (e.g. by a
// playback banner), so the picker only closes.
export function PresetPicker({
    onSave,
    mappedSlot = null,
    label = 'Save as preset',
    title,
    wrapClass = 'preset-picker-wrap',
    buttonClass = 'preset-picker-btn',
    overlayClass = 'preset-picker-overlay',
}) {
    const [open, setOpen] = useState(false);
    const [savingSlot, setSavingSlot] = useState(null);
    const [savedSlot, setSavedSlot] = useState(null);
    const [errorSlot, setErrorSlot] = useState(null);
    const wrapRef = useRef(null);

    // Close the popover on an outside click.
    useEffect(() => {
        if (!open) return;

        function onDocClick(e) {
            if (!wrapRef.current?.contains(e.target)) setOpen(false);
        }

        document.addEventListener('click', onDocClick, true);

        return () => document.removeEventListener('click', onDocClick, true);
    }, [open]);

    // Mirrors the feedback the preset tiles give (see Presets.js): a failed
    // save used to close the overlay silently, which looked identical to a
    // successful one.
    function finish(setter, slotId) {
        setSavingSlot(null);
        setter(slotId);
        setTimeout(() => {
            setter(null);
            setOpen(false);
        }, 900);
    }

    function save(slotId) {
        const pending = onSave?.(slotId);

        if (!pending?.then) {
            // The caller reports the outcome itself.
            setOpen(false);

            return;
        }

        setSavingSlot(slotId);
        pending
            .then(() => finish(setSavedSlot, slotId))
            .catch(() => finish(setErrorSlot, slotId));
    }

    const isMapped = mappedSlot !== null && mappedSlot !== undefined;
    const buttonTitle = title || (isMapped
        ? `Saved as preset ${mappedSlot}. Save again to update.`
        : label);

    return html`
        <div class=${wrapClass} ref=${wrapRef}>
            <button
                class="${buttonClass} ${isMapped ? 'mapped' : ''} ${open ? 'open' : ''}"
                onClick=${e => { e.stopPropagation(); setOpen(o => !o); }}
                title=${buttonTitle}
                aria-label=${buttonTitle}
            >★</button>
            ${open && html`
                <div class=${overlayClass}>
                    <div class="preset-picker-label">${label}</div>
                    <div class="preset-picker-slots">
                        ${SLOTS.map(n => html`
                            <button
                                key=${n}
                                class="preset-picker-slot ${savingSlot === n ? 'saving' : savedSlot === n ? 'saved' : errorSlot === n ? 'error' : ''}"
                                onClick=${e => { e.stopPropagation(); save(n); }}
                                disabled=${savingSlot !== null}
                            >${savedSlot === n ? '✓' : errorSlot === n ? '✗' : n}</button>
                        `)}
                    </div>
                </div>
            `}
        </div>
    `;
}
