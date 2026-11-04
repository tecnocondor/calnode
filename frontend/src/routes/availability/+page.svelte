<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AvailabilityRule, type AvailabilityOverride } from '$lib/api';
	import { prefs, fmtDate, weekDays } from '$lib/prefs';
	import { t, tr, localeTag } from '$lib/i18n';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { ConfirmDialog } from '$lib/components/ui/confirm-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import * as Select from '$lib/components/ui/select';
	import { DatePicker } from '$lib/components/ui/date-picker';

	import WeeklyHours from '$lib/components/availability/WeeklyHours.svelte';

	const TIME_SLOTS: string[] = [];
	for (let h = 0; h < 24; h++) {
		for (const m of [0, 30]) {
			TIME_SLOTS.push(`${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`);
		}
	}

	function fmtTime(slot: string) {
		const [hh, mm] = slot.split(':').map(Number);
		const date = new Date(2000, 0, 1, hh, mm);
		if ($prefs.time_format === '24h') {
			return date.toLocaleTimeString(localeTag(), { hour: '2-digit', minute: '2-digit', hour12: false });
		}
		return date.toLocaleTimeString(localeTag(), {
			hour: 'numeric',
			minute: mm === 0 ? undefined : '2-digit',
			hour12: true
		});
	}

	// ── Date overrides ────────────────────────────────────────────────────────────
	let overrides: AvailabilityOverride[] = $state([]);
	let overridesLoading = $state(true);
	let overridesError = $state('');

	type OverrideReason = 'day_off' | 'out_of_office' | 'custom_hours';
	const REASON_LABELS = $derived<Record<OverrideReason, string>>({
		day_off: $t('availability.override.type.dayOff'),
		out_of_office: $t('availability.override.type.outOfOffice'),
		custom_hours: $t('availability.override.type.customHours')
	});

	let ovForm = $state({ date: '', end_date: '', reason: 'day_off' as OverrideReason, start_time: '09:00', end_time: '17:00' });
	let addingOv = $state(false);
	let ovAddError = $state('');

	let deleteOvOpen = $state(false);
	let deleteOvId = $state('');

	let deleteGroupId = $state('');
	let deleteGroupOpen = $state(false);

	// Collapse per-date rows that share a group_id (a multi-day span) into one entry.
	type OvEntry =
		| { kind: 'single'; ov: AvailabilityOverride }
		| { kind: 'span'; group_id: string; reason: OverrideReason; start: string; end: string; days: number };
	const overrideEntries = $derived.by((): OvEntry[] => {
		const groups = new Map<string, AvailabilityOverride[]>();
		const entries: OvEntry[] = [];
		for (const ov of overrides) {
			if (ov.group_id) {
				const arr = groups.get(ov.group_id) ?? [];
				arr.push(ov);
				groups.set(ov.group_id, arr);
			} else {
				entries.push({ kind: 'single', ov });
			}
		}
		for (const [gid, arr] of groups) {
			const dates = arr.map((o) => o.date).sort();
			entries.push({
				kind: 'span',
				group_id: gid,
				reason: (arr[0].reason ?? 'out_of_office') as OverrideReason,
				start: dates[0],
				end: dates[dates.length - 1],
				days: dates.length
			});
		}
		return entries.sort((a, b) =>
			(a.kind === 'single' ? a.ov.date : a.start).localeCompare(b.kind === 'single' ? b.ov.date : b.start)
		);
	});

	async function loadOverrides() {
		overridesError = '';
		try {
			const res = await api.get<{ items: AvailabilityOverride[] }>('/v1/availability-overrides');
			overrides = (res.items ?? []).sort((a, b) => a.date.localeCompare(b.date));
		} catch (e: any) {
			overridesError = e.message;
		} finally {
			overridesLoading = false;
		}
	}

	// Dates whose custom-hours blocks overlap: they merge into one window at slot
	// time, so flag them rather than silently showing two rows that act as one.
	const overlappingOverrideDates = $derived.by((): string[] => {
		const byDate = new Map<string, AvailabilityOverride[]>();
		for (const ov of overrides) {
			if (ov.reason !== 'custom_hours' || !ov.start_time || !ov.end_time) continue;
			const arr = byDate.get(ov.date) ?? [];
			arr.push(ov);
			byDate.set(ov.date, arr);
		}
		const out: string[] = [];
		for (const [date, blocks] of byDate) {
			const sorted = [...blocks].sort((a, b) => (a.start_time ?? '').localeCompare(b.start_time ?? ''));
			for (let i = 1; i < sorted.length; i++) {
				if ((sorted[i].start_time ?? '') < (sorted[i - 1].end_time ?? '')) {
					out.push(date);
					break;
				}
			}
		}
		return out;
	});



	async function addOverride() {
		ovAddError = '';
		if (!ovForm.date) { ovAddError = tr('availability.validation.dateRequired'); return; }
		if (ovForm.reason === 'custom_hours' && (!ovForm.start_time || !ovForm.end_time)) {
			ovAddError = tr('availability.validation.customHoursTimes'); return;
		}
		if (ovForm.reason === 'custom_hours' && ovForm.start_time >= ovForm.end_time) {
			ovAddError = tr('availability.validation.endAfterStart'); return;
		}
		if (ovForm.end_date && ovForm.end_date < ovForm.date) {
			ovAddError = tr('availability.validation.endDate'); return;
		}
		addingOv = true;
		try {
			const isRange = ovForm.reason !== 'custom_hours' && !!ovForm.end_date && ovForm.end_date > ovForm.date;
			await api.post('/v1/availability-overrides', {
				date: ovForm.date,
				reason: ovForm.reason,
				...(ovForm.reason === 'custom_hours'
					? { start_time: ovForm.start_time, end_time: ovForm.end_time }
					: {}),
				...(isRange ? { end_date: ovForm.end_date } : {})
			});
			ovForm = { date: '', end_date: '', reason: 'day_off', start_time: '09:00', end_time: '17:00' };
			await loadOverrides();
		} catch (e: any) {
			ovAddError = e.message;
		} finally {
			addingOv = false;
		}
	}

	function deleteOverride(id: string) {
		deleteOvId = id;
		deleteOvOpen = true;
	}

	async function doDeleteOverride() {
		try {
			await api.del(`/v1/availability-overrides/${deleteOvId}`);
			await loadOverrides();
		} catch (e: any) {
			overridesError = e.message;
		}
	}

	function deleteGroup(groupId: string) {
		deleteGroupId = groupId;
		deleteGroupOpen = true;
	}

	async function doDeleteGroup() {
		try {
			await api.del(`/v1/availability-overrides/group/${deleteGroupId}`);
			await loadOverrides();
		} catch (e: any) {
			overridesError = e.message;
		}
	}

	onMount(() => {
		loadOverrides();
	});

	const REASON_OPTIONS = $derived<{ value: OverrideReason; label: string }[]>([
		{ value: 'day_off', label: REASON_LABELS.day_off },
		{ value: 'out_of_office', label: REASON_LABELS.out_of_office },
		{ value: 'custom_hours', label: REASON_LABELS.custom_hours },
	]);

	// Time-of-day options for the start/end selects, formatted per the user's prefs.
	let timeOptions = $derived(TIME_SLOTS.map((t) => ({ value: t, label: fmtTime(t) })));
	function timeLabel(t: string) { return fmtTime(t); }
</script>

<ConfirmDialog
	bind:open={deleteOvOpen}
	title={$t('availability.confirmRemoveOverride.title')}
	description={$t('availability.confirmRemoveOverride.description')}
	confirmText={$t('common.remove')}
	destructive
	onConfirm={doDeleteOverride}
/>

<ConfirmDialog
	bind:open={deleteGroupOpen}
	title={$t('availability.confirmRemoveSpan.title')}
	description={$t('availability.confirmRemoveSpan.description')}
	confirmText={$t('common.remove')}
	destructive
	onConfirm={doDeleteGroup}
/>

<svelte:head><title>{$t('availability.headTitle')}</title></svelte:head>

<div class="mb-8">
	<h1 class="text-2xl font-semibold tracking-tight">{$t('availability.title')}</h1>
	<p class="mt-1 text-sm text-muted-foreground">{$t('availability.subtitle')}</p>
</div>

<!-- Weekly Hours -->
<div class="mb-8">
	<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{$t('availability.weeklyHours')}</h2>

	<WeeklyHours />
</div>

<!-- Date Overrides -->
<div>
	<h2 class="mb-1 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{$t('availability.overrides.title')}</h2>
	<p class="mb-3 text-sm text-muted-foreground">{$t('availability.overrides.subtitle')}</p>

	<div class="rounded-lg border bg-card">
		{#if overridesError}<p class="px-4 pt-4 text-sm text-destructive">{overridesError}</p>{/if}
		{#if overlappingOverrideDates.length > 0}
			<p class="px-4 pt-4 text-xs text-amber-700">{$t('availability.overrides.overlapWarning', { dates: overlappingOverrideDates.map((d) => fmtDate(d, $prefs)).join(', ') })}</p>
		{/if}

		{#if overridesLoading}
			<p class="px-4 py-4 text-sm text-muted-foreground">{$t('common.loading')}</p>
		{:else if overrides.length > 0}
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b">
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">{$t('availability.overrides.table.date')}</th>
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">{$t('availability.overrides.table.type')}</th>
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">{$t('availability.overrides.table.duration')}</th>
						<th class="px-4 pb-3 pt-3"></th>
					</tr>
				</thead>
				<tbody class="divide-y">
					{#each overrideEntries as entry (entry.kind === 'span' ? entry.group_id : entry.ov.id)}
						{#if entry.kind === 'span'}
							<tr class="transition-colors hover:bg-muted/30">
								<td class="px-4 py-3 font-medium">{fmtDate(entry.start, $prefs)} – {fmtDate(entry.end, $prefs)}</td>
								<td class="px-4 py-3">
									{#if entry.reason === 'out_of_office'}
										<Badge class="bg-amber-50 text-amber-700 border-amber-200">{REASON_LABELS.out_of_office}</Badge>
									{:else}
										<Badge variant="secondary">{REASON_LABELS.day_off}</Badge>
									{/if}
								</td>
								<td class="px-4 py-3 text-muted-foreground">{$t('availability.overrides.duration.days', { count: entry.days })}</td>
								<td class="px-4 py-3">
									<Tooltip.Provider>
										<div class="flex items-center justify-end gap-1">
											<Tooltip.Root>
												<Tooltip.Trigger class={buttonVariants({ variant: 'ghost', size: 'icon' })} onclick={() => deleteGroup(entry.group_id)}>
													<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/></svg>
												</Tooltip.Trigger>
												<Tooltip.Content>{$t('availability.tooltip.deleteSpan')}</Tooltip.Content>
											</Tooltip.Root>
										</div>
									</Tooltip.Provider>
								</td>
							</tr>
						{:else}
							{@const ov = entry.ov}
							<tr class="transition-colors hover:bg-muted/30">
								<td class="px-4 py-3 font-medium">{fmtDate(ov.date, $prefs)}</td>
								<td class="px-4 py-3">
									{#if ov.reason === 'custom_hours'}
										<Badge class="bg-blue-50 text-blue-700 border-blue-200">{REASON_LABELS.custom_hours}</Badge>
									{:else if ov.reason === 'out_of_office'}
										<Badge class="bg-amber-50 text-amber-700 border-amber-200">{REASON_LABELS.out_of_office}</Badge>
									{:else}
										<Badge variant="secondary">{REASON_LABELS.day_off}</Badge>
									{/if}
								</td>
								<td class="px-4 py-3 text-muted-foreground">
									{ov.is_available && ov.start_time && ov.end_time
										? `${fmtTime(ov.start_time)} – ${fmtTime(ov.end_time)}`
										: $t('availability.overrides.duration.oneDay')}
								</td>
								<td class="px-4 py-3">
									<Tooltip.Provider>
										<div class="flex items-center justify-end gap-1">
											<Tooltip.Root>
												<Tooltip.Trigger
													class={buttonVariants({ variant: 'ghost', size: 'icon' })}
													onclick={() => deleteOverride(ov.id)}
												>
													<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/></svg>
												</Tooltip.Trigger>
												<Tooltip.Content>{$t('availability.tooltip.delete')}</Tooltip.Content>
											</Tooltip.Root>
										</div>
									</Tooltip.Provider>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="px-4 py-4 text-sm text-muted-foreground">{$t('availability.overrides.empty')}</p>
		{/if}

		<!-- Add override form -->
		<div class="border-t px-4 py-4">
			{#if ovAddError}<p class="mb-3 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{ovAddError}</p>{/if}
			<div class="flex flex-wrap items-end gap-3">
				<div class="space-y-1.5">
					<label for="ov-date" class="text-sm font-medium">{$t('availability.overrides.form.date')}</label>
					<DatePicker bind:value={ovForm.date} placeholder={$t('common.datePicker.pickDate')} />
				</div>
				{#if ovForm.reason !== 'custom_hours'}
					<div class="space-y-1.5">
						<label for="ov-end-date" class="text-sm font-medium">{$t('availability.overrides.form.toOptional')}</label>
						<DatePicker bind:value={ovForm.end_date} placeholder={$t('common.datePicker.sameDay')} />
					</div>
				{/if}
				<div class="space-y-1.5">
					<label for="ov-type" class="text-sm font-medium">{$t('availability.overrides.form.type')}</label>
					<Select.Root type="single" value={ovForm.reason} onValueChange={(v) => { if (v) ovForm.reason = v as OverrideReason; }}>
						<Select.Trigger id="ov-type" class="w-fit">{REASON_LABELS[ovForm.reason]}</Select.Trigger>
						<Select.Content>
							{#each REASON_OPTIONS as r}
								<Select.Item value={r.value} label={r.label}>{r.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				{#if ovForm.reason === 'custom_hours'}
					<div class="space-y-1.5">
						<label for="ov-start" class="text-sm font-medium">{$t('availability.overrides.form.from')}</label>
						<Select.Root type="single" bind:value={ovForm.start_time}>
							<Select.Trigger id="ov-start" class="w-fit">{timeLabel(ovForm.start_time)}</Select.Trigger>
							<Select.Content>
								{#each timeOptions as opt}<Select.Item value={opt.value} label={opt.label}>{opt.label}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<div class="space-y-1.5">
						<label for="ov-end" class="text-sm font-medium">{$t('availability.overrides.form.to')}</label>
						<Select.Root type="single" bind:value={ovForm.end_time}>
							<Select.Trigger id="ov-end" class="w-fit">{timeLabel(ovForm.end_time)}</Select.Trigger>
							<Select.Content>
								{#each timeOptions as opt}<Select.Item value={opt.value} label={opt.label}>{opt.label}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
					</div>
				{/if}
				<Button onclick={addOverride} disabled={addingOv}>
					{addingOv ? $t('availability.overrides.adding') : $t('availability.overrides.add')}
				</Button>
				{#if ovForm.reason === 'custom_hours'}
					<p class="w-full text-xs text-muted-foreground">{$t('availability.overrides.splitHint')}</p>
				{/if}
			</div>
		</div>
	</div>
</div>
