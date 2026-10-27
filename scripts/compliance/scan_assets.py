#!/usr/bin/env python3
"""Build a reproducible first-party portal software-license inventory.

This is an evidence collector, not an automated legal clearance tool. Unknown
ownership and permission are deliberately retained as unknown.
"""
from __future__ import annotations
import argparse, csv, hashlib, json, re, subprocess
from pathlib import Path, PurePosixPath
from datetime import datetime, timezone

EXT_TYPES = {
    '.svg': 'icon', '.png': 'image', '.jpg': 'image', '.jpeg': 'image', '.webp': 'image', '.gif': 'image', '.avif': 'image', '.ico': 'image',
    '.woff': 'font', '.woff2': 'font', '.ttf': 'font', '.otf': 'font', '.eot': 'font',
    '.mp3': 'audio', '.wav': 'audio', '.ogg': 'audio', '.mp4': 'video', '.webm': 'video',
    '.css': 'source_file', '.js': 'source_file', '.mjs': 'source_file', '.ts': 'source_file', '.vue': 'source_file', '.html': 'source_file', '.json': 'source_file', '.txt': 'policy_document',
}
LICENSE_NAMES = {'license','license.md','license.txt','license.rst','copying','copying.md','notice','notice.md','notice.txt','license-mit','license-apache'}

def sha_bytes(b: bytes) -> str: return hashlib.sha256(b).hexdigest()
def sha_file(p: Path) -> str:
    h=hashlib.sha256()
    with p.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024), b''): h.update(chunk)
    return h.hexdigest()
def sqlq(v):
    if v is None: return 'NULL'
    if isinstance(v, bool): return 'TRUE' if v else 'FALSE'
    if isinstance(v, (int,float)): return str(v)
    return "'" + str(v).replace("'", "''") + "'"
def sqljson(v): return sqlq(json.dumps(v,ensure_ascii=False,separators=(',',':'))) + '::jsonb'
def package_name(lock_path):
    tail=lock_path.rsplit('node_modules/',1)[-1]
    bits=tail.split('/')
    return '/'.join(bits[:2]) if bits and bits[0].startswith('@') and len(bits)>1 else bits[0]
def path_is_within_root(root: Path, candidate: Path):
    try:
        root_path=Path(root).resolve(strict=True)
        candidate_path=Path(candidate)
        if candidate_path.exists() or candidate_path.is_symlink():
            candidate_resolved=candidate_path.resolve(strict=True)
        else:
            missing_parts=[]
            ancestor=candidate_path
            while not ancestor.exists() and not ancestor.is_symlink():
                missing_parts.append(ancestor.name)
                ancestor=ancestor.parent
            ancestor_resolved=ancestor.resolve(strict=True)
            candidate_resolved=ancestor_resolved.joinpath(*reversed(missing_parts))
    except (OSError, RuntimeError):
        return False
    return candidate_resolved.is_relative_to(root_path)
def safe_package_directory(frontend_root: Path, lock_path: str):
    relative=PurePosixPath(lock_path)
    if relative.is_absolute() or '..' in relative.parts or '\\' in lock_path:
        raise ValueError('unsafe package-lock path: '+lock_path)
    candidate=Path(frontend_root).joinpath(*relative.parts)
    if not path_is_within_root(frontend_root,candidate):
        raise ValueError('package path escapes frontend root: '+lock_path)
    return candidate
def current_asset_source(assets):
    current_pairs=','.join('('+sqlq(a['asset_path'])+','+sqlq(a['sha256'])+')' for a in assets)
    if current_pairs:
        return '(VALUES '+current_pairs+')'
    return '(SELECT NULL::text AS asset_path, NULL::text AS sha256 WHERE FALSE)'
def git_source_state(repo: Path):
    revision=subprocess.run(['git','-C',str(repo),'rev-parse','HEAD'],capture_output=True,text=True,check=True).stdout.strip()
    status=subprocess.run(['git','-C',str(repo),'status','--porcelain'],capture_output=True,text=True,check=True).stdout
    return revision, bool(status.strip())
def resolve_dep(parent, name, packages):
    base=parent
    while base:
        candidate=base.rstrip('/')+'/node_modules/'+name
        if candidate in packages: return candidate
        parent_base=base.rsplit('/',1)[0] if '/' in base else ''
        if parent_base == 'node_modules': parent_base=''
        base=parent_base
    candidate='node_modules/'+name
    return candidate if candidate in packages else None
def production_closure(lock):
    packages=lock.get('packages',{}); root=packages.get('',{})
    todo=[]
    for group in ('dependencies','optionalDependencies'):
        todo += [(None,n) for n in root.get(group,{})]
    seen=set()
    while todo:
        parent,name=todo.pop()
        key=('node_modules/'+name) if parent is None else resolve_dep(parent,name,packages)
        if not key or key not in packages or key in seen: continue
        seen.add(key); meta=packages[key]
        for group in ('dependencies','optionalDependencies','peerDependencies'):
            for dep in meta.get(group,{}): todo.append((key,dep))
    return packages, seen
def find_license_file(pkgdir: Path):
    try:
        files = [
            p for p in pkgdir.iterdir()
            if p.name.lower() in LICENSE_NAMES and p.is_file() and path_is_within_root(pkgdir, p)
        ]
    except (OSError, RuntimeError):
        return None
    files.sort(key=lambda p:(0 if p.name.lower() in ('license','license.md','license.txt') else 1,len(p.name)))
    return files[0] if files else None
def license_family(expr):
    s=(expr or '').lower()
    if ' OR ' in s.upper() or ' AND ' in s.upper(): return 'other'
    if any(x in s for x in ('agpl','gpl','lgpl','mpl','epl','cddl','sspl')):
        return 'copyleft' if any(x in s for x in ('agpl','gpl')) and 'lgpl' not in s else 'weak_copyleft'
    if any(x in s for x in ('mit','apache','bsd','isc','0bsd','unlicense','cc0','zlib','artistic-2.0')): return 'permissive'
    if any(x in s for x in ('proprietary','commercial','license-ref','licenseref','n8n-enterprise')): return 'proprietary'
    if 'public domain' in s or 'public-domain' in s: return 'public_domain'
    return 'other'
def canonical_url(expr):
    s=(expr or '').strip()
    urls={'MIT':'https://spdx.org/licenses/MIT.html','Apache-2.0':'https://spdx.org/licenses/Apache-2.0.html','BSD-3-Clause':'https://spdx.org/licenses/BSD-3-Clause.html','BSD-2-Clause':'https://spdx.org/licenses/BSD-2-Clause.html','ISC':'https://spdx.org/licenses/ISC.html','LGPL-3.0-or-later':'https://spdx.org/licenses/LGPL-3.0-or-later.html','GPL-3.0-or-later':'https://spdx.org/licenses/GPL-3.0-or-later.html','MPL-2.0':'https://spdx.org/licenses/MPL-2.0.html','CC0-1.0':'https://spdx.org/licenses/CC0-1.0.html'}
    return urls.get(s,'')
def make_asset(**kw):
    defaults=dict(asset_type='other',asset_name='',asset_path='',source_url='',creator_or_rightsholder='',ownership_basis='unknown',license_key=None,permission_status='unknown',use_scope='unknown',distribution_status='unknown',review_status='pending',risk_level='unknown',sha256='',file_size_bytes=None,discovered_by='manual',published_or_active=False,metadata={},notes='')
    defaults.update(kw); return defaults

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument('--repo',default='/opt/personal-portal')
    ap.add_argument('--release',default='/usr/local/www/personal-portal/current')
    ap.add_argument('--out',default='docs/compliance'); ap.add_argument('--strict-licenses',action='store_true')
    args=ap.parse_args(); repo=Path(args.repo); out=repo/args.out
    source_revision, source_worktree_dirty=git_source_state(repo)
    out.mkdir(parents=True,exist_ok=True)
    try: report_path=str((out/'asset-inventory.json').relative_to(repo))
    except ValueError: report_path=str(out/'asset-inventory.json')
    now=datetime.now(timezone.utc).isoformat()
    assets=[]; licenses={}; evidence=[]; prod_rows=[]
    def add_license(expr, version='', name=None, text='', notes=''):
        expr=(expr or '').strip()
        if not expr: expr='LicenseRef:unknown'
        fam=license_family(expr)
        lv=version if expr.startswith('LicenseRef:') else ''
        key=(expr,lv)
        item=licenses.setdefault(key,dict(spdx_id=expr,license_name=name or expr,license_version=lv,license_family=fam,canonical_url=canonical_url(expr),full_text='',commercial_use_allowed=(True if fam in ('permissive','public_domain') else False if fam=='proprietary' else None),modification_allowed=(True if fam in ('permissive','public_domain') else None),redistribution_allowed=(True if fam in ('permissive','public_domain') else None),attribution_required=(True if fam=='permissive' else None),source_code_offer_required=(True if fam in ('copyleft','weak_copyleft') else None),review_status='unreviewed',notes=''))
        if text and not item['full_text']: item['full_text']=text[:200000]
        if notes and notes not in item['notes']: item['notes']=(item['notes']+'; '+notes).strip('; ')
        return key
    def add_file(p: Path, path_label: str, distribution: str, active: bool, discovered='filesystem_scan', allowed_root: Path | None=None):
        try:
            if not p.is_file(): return
            if allowed_root is not None and not path_is_within_root(allowed_root,p):
                assets.append(make_asset(asset_name=p.name,asset_path=path_label,discovered_by=discovered,metadata={'scan_error':'resolved file path escapes the configured scan root'},notes='Path target is outside the configured scan root; file content was not read and requires manual follow-up.'))
                return
            resolved=p.resolve(strict=True)
            st=resolved.stat(); ext=p.suffix.lower(); typ=EXT_TYPES.get(ext,'other')
            if 'icons/bootstrap-icons/' in path_label.replace('\\','/') and ext=='.svg': typ='icon'
            sha=sha_file(resolved)
            lic_key=None; own='unknown'; perm='unknown'; risk='unknown'; notes='Automated file inventory; ownership and authorization have not been legally verified.'
            meta={'extension':ext,'scan_timestamp':now,'source_relative_path':path_label,'sha256_method':'SHA-256'}
            if typ=='icon' and 'icons/bootstrap-icons/' in path_label.replace('\\','/'):
                lic_key=add_license('MIT',name='MIT License',notes='Bootstrap Icons artwork; exact notice preserved in frontend/public/THIRD_PARTY_NOTICES.txt')
                own='open_license'; perm='conditional'; risk='low'; meta.update({'upstream':'twbs/icons','upstream_version':'1.13.2','notice_path':'frontend/public/THIRD_PARTY_NOTICES.txt'})
                notes='MIT license declared in bundled third-party notices; verify notice remains included in every distribution.'
            elif typ=='source_file' and path_label.startswith('frontend/src/'):
                own='original'; risk='unknown'; meta['source_role']='first_party_project_source'; notes='Project source file; legal rights holder and contributor chain not independently verified.'
            assets.append(make_asset(asset_type=typ,asset_name=p.name,asset_path=path_label,ownership_basis=own,license_key=lic_key,permission_status=perm,distribution_status=distribution,review_status='pending',risk_level=risk,sha256=sha,file_size_bytes=st.st_size,discovered_by=discovered,published_or_active=active,metadata=meta,notes=notes))
        except (OSError, RuntimeError) as e:
            assets.append(make_asset(asset_name=p.name,asset_path=path_label,discovered_by=discovered,metadata={'scan_error':str(e)},notes='File could not be fully inspected; requires manual follow-up.'))
    # Project-owned active source files; exclude historical .bak copies from active asset counts.
    src=repo/'frontend/src'
    for p in sorted(src.rglob('*')):
        if p.is_file() and '.bak-' not in p.name and not p.name.endswith('.bak') and p.suffix.lower() in {'.svg','.png','.jpg','.jpeg','.webp','.gif','.ico','.woff','.woff2','.ttf','.otf','.css','.js','.mjs','.ts','.vue','.html','.json'}:
            add_file(p, str(p.relative_to(repo)), 'unknown', False, allowed_root=src)
    # Public source assets and currently deployed assets.
    for base, label, dist, active in [(repo/'frontend/public','frontend/public','bundled_in_frontend',True),(Path(args.release),'deployed/current','served_publicly',True)]:
        if not base.exists(): continue
        for p in sorted(base.rglob('*')):
            if p.is_file():
                try: rel=p.relative_to(base).as_posix()
                except ValueError: rel=p.name
                add_file(p, f'{label}/{rel}', dist, active, 'filesystem_scan', allowed_root=base)
    # Dependency license records from the lockfile, restricted to production closure.
    lock=json.loads((repo/'frontend/package-lock.json').read_text()); packages, prod=production_closure(lock)
    notice=(repo/'frontend/public/THIRD_PARTY_NOTICES.txt').read_text(errors='replace') if (repo/'frontend/public/THIRD_PARTY_NOTICES.txt').exists() else ''
    notice_sections=set(re.findall(r'^### (.+?)@([^\n]+)$',notice,re.M))
    missing_metadata=[]; missing_license_file=[]; missing_notice=[]
    frontend_root=repo/'frontend'
    for key in sorted(prod):
        meta=packages[key]; name=package_name(key); ver=meta.get('version',''); expr=meta.get('license')
        if isinstance(expr,dict): expr=expr.get('type') or json.dumps(expr,ensure_ascii=False)
        if isinstance(expr,list): expr=' OR '.join(str(x) for x in expr)
        try:
            pkgdir=safe_package_directory(frontend_root,key)
        except ValueError as e:
            raise SystemExit(str(e)) from e
        licfile=find_license_file(pkgdir); lictext=''
        if licfile:
            try: lictext=licfile.read_text(errors='replace')[:200000]
            except (OSError, RuntimeError): lictext=''
        if not expr:
            missing_metadata.append(f'{name}@{ver}')
            expr='LicenseRef:'+name
        if not licfile: missing_license_file.append(f'{name}@{ver}')
        if (name,ver) not in notice_sections: missing_notice.append(f'{name}@{ver}')
        lkey=add_license(expr,name=expr,text=lictext,notes=f'Observed in production dependency closure: {name}@{ver}; package metadata is not a legal determination.')
        license_sha=sha_bytes(lictext.encode()) if lictext else ''
        asset=make_asset(asset_type='software_dependency',asset_name=f'{name}@{ver}',asset_path=f'npm://{name}@{ver}#{key}',source_url=canonical_url(expr),ownership_basis='open_license' if not expr.startswith('LicenseRef:') else 'unknown',license_key=lkey,permission_status='conditional' if lictext else 'unknown',use_scope='commercial_web' if license_family(expr)=='permissive' else 'unknown',distribution_status='unknown',review_status='pending',risk_level=('high' if any(x in expr.lower() for x in ('agpl','gpl','lgpl','n8n-enterprise','proprietary','licenseref')) else 'medium' if (' OR ' in expr.upper() or ' AND ' in expr.upper() or 'mpl-' in expr.lower()) else 'unknown' if expr.startswith('LicenseRef:') else 'low'),discovered_by='package_lock_scan',published_or_active=True,metadata={'package_name':name,'version':ver,'lockfile_path':key,'license_expression':expr,'license_file':str(licfile.relative_to(repo)) if licfile else None,'license_file_sha256':license_sha or None,'license_file_found':bool(licfile),'third_party_notice_section_found':(name,ver) in notice_sections,'distribution_verified':False,'dependency_scope':'production_closure'},notes='License metadata and local license file are evidence only; actual bundling and applicable obligations require review.')
        assets.append(asset)
        prod_rows.append({'package':name,'version':ver,'license_expression':expr,'license_file':str(licfile.relative_to(repo)) if licfile else '', 'license_file_sha256':license_sha,'license_text_found':bool(lictext),'notice_section_found':(name,ver) in notice_sections,'lockfile_path':key,'risk_triage':asset['risk_level']})
        if licfile:
            evidence.append({'asset_path':asset['asset_path'],'evidence_type':'license_text','evidence_uri':str(licfile.relative_to(repo)),'evidence_text':f'Package metadata declares {expr}; local license file collected. Hash is evidence of captured bytes, not legal approval.','sha256':license_sha,'verified':False,'notes':'Automated package metadata/license-file capture.'})
    # Preserve the exact Bootstrap Icons copyright/permission notice as evidence.
    icon_notice=''
    if notice:
        start=notice.find('## Embedded Bootstrap Icons artwork')
        end=notice.find('## Production dependency packages', start + 1)
        if start >= 0:
            icon_notice=notice[start:end if end >= 0 else len(notice)].strip()
    if icon_notice:
        icon_hash=sha_bytes(icon_notice.encode())
        for a in assets:
            if a['asset_type']=='icon' and a.get('metadata',{}).get('upstream')=='twbs/icons':
                evidence.append({'asset_path':a['asset_path'],'evidence_type':'license_text','evidence_uri':'frontend/public/THIRD_PARTY_NOTICES.txt','evidence_text':icon_notice,'sha256':icon_hash,'verified':False,'notes':'Exact upstream notice captured from the current public third-party notice file; license compliance still requires review.'})
    # Existing third-party notices file is itself a compliance artifact.
    notice_path=repo/'frontend/public/THIRD_PARTY_NOTICES.txt'
    if notice_path.is_file():
        assets.append(make_asset(asset_type='policy_document',asset_name='THIRD_PARTY_NOTICES.txt',asset_path='report://THIRD_PARTY_NOTICES.txt',ownership_basis='original',permission_status='conditional',distribution_status='bundled_in_frontend',review_status='pending',risk_level='low',sha256=sha_file(notice_path),file_size_bytes=notice_path.stat().st_size,discovered_by='filesystem_scan',published_or_active=True,metadata={'notice_sections':len(notice_sections),'production_dependency_closure':len(prod),'missing_notice_sections':missing_notice,'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file},notes='Contains Bootstrap Icons MIT notice and production dependency license text; verify section coverage and source notice fidelity.'))
    # Write inventory exports.
    def clean_asset(a):
        x=dict(a); x['license_key']=list(x['license_key']) if x.get('license_key') else None; return x
    (out/'asset-inventory.json').write_text(json.dumps({'generated_at':now,'source_revision':source_revision,'source_worktree_dirty':source_worktree_dirty,'assets':[clean_asset(a) for a in assets]},ensure_ascii=False,indent=2)+'\n')
    with (out/'asset-inventory.csv').open('w',newline='',encoding='utf-8-sig') as f:
        fields=['asset_type','asset_name','asset_path','source_url','ownership_basis','permission_status','use_scope','distribution_status','review_status','risk_level','sha256','file_size_bytes','discovered_by','published_or_active','notes','metadata']
        w=csv.DictWriter(f,fieldnames=fields); w.writeheader()
        for a in assets:
            row={k:a.get(k) for k in fields}; row['metadata']=json.dumps(row['metadata'],ensure_ascii=False); w.writerow(row)
    with (out/'third-party-license-register.csv').open('w',newline='',encoding='utf-8-sig') as f:
        fields=['package','version','license_expression','license_file','license_file_sha256','license_text_found','notice_section_found','lockfile_path','risk_triage']; w=csv.DictWriter(f,fieldnames=fields); w.writeheader(); w.writerows(prod_rows)
    # SQL import/upsert script; no license is marked legally approved by automation.
    sql=['-- Generated by scripts/compliance/scan_assets.py. Evidence capture only; not legal clearance.','-- Source revision: '+source_revision+'; worktree_dirty='+str(source_worktree_dirty).lower(),'BEGIN;']
    for l in licenses.values():
        sql.append(
            'INSERT INTO portal.compliance_license_catalog (spdx_id,license_name,license_version,license_family,canonical_url,full_text,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status,notes) VALUES (' + ','.join([sqlq(l['spdx_id']),sqlq(l['license_name']),sqlq(l['license_version']),sqlq(l['license_family']),sqlq(l['canonical_url']),sqlq(l['full_text']),sqlq(l['commercial_use_allowed']),sqlq(l['modification_allowed']),sqlq(l['redistribution_allowed']),sqlq(l['attribution_required']),sqlq(l['source_code_offer_required']),sqlq('unreviewed'),sqlq(l['notes'])]) +
            ") ON CONFLICT (spdx_id,license_version) DO UPDATE SET canonical_url=CASE WHEN length(portal.compliance_license_catalog.canonical_url)=0 THEN EXCLUDED.canonical_url ELSE portal.compliance_license_catalog.canonical_url END, full_text=CASE WHEN length(portal.compliance_license_catalog.full_text)=0 THEN EXCLUDED.full_text ELSE portal.compliance_license_catalog.full_text END, license_family=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.license_family ELSE portal.compliance_license_catalog.license_family END, commercial_use_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.commercial_use_allowed ELSE portal.compliance_license_catalog.commercial_use_allowed END, modification_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.modification_allowed ELSE portal.compliance_license_catalog.modification_allowed END, redistribution_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.redistribution_allowed ELSE portal.compliance_license_catalog.redistribution_allowed END, attribution_required=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.attribution_required ELSE portal.compliance_license_catalog.attribution_required END, source_code_offer_required=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.source_code_offer_required ELSE portal.compliance_license_catalog.source_code_offer_required END, notes=CASE WHEN length(portal.compliance_license_catalog.notes)=0 THEN EXCLUDED.notes ELSE portal.compliance_license_catalog.notes END, updated_at=now();"
        )
    current_source=current_asset_source(assets)
    finding_count=len([a for a in assets if a['risk_level'] in ('high','critical','unknown')])
    scan_summary={'generated_at':now,'source_revision':source_revision,'source_worktree_dirty':source_worktree_dirty,'production_dependency_count':len(prod),'license_catalog_records':len(licenses),'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file,'missing_notice_sections':missing_notice,'risk_counts':{k:sum(1 for a in assets if a['risk_level']==k) for k in ('critical','high','medium','low','unknown')}}
    scan_sql=("INSERT INTO portal.compliance_scan_runs(scan_type,scan_status,source_revision,assets_seen,assets_created,assets_updated,findings_count,report_path,summary,completed_at) SELECT 'full_audit','completed_with_warnings',"+sqlq(source_revision)+","+str(len(assets))+",COUNT(*) FILTER (WHERE existing.asset_id IS NULL),COUNT(*) FILTER (WHERE existing.asset_id IS NOT NULL),"+str(finding_count)+","+sqlq(report_path)+","+sqlq(json.dumps(scan_summary,ensure_ascii=False))+"::jsonb,now() FROM "+current_source+" AS current_asset(asset_path,sha256) LEFT JOIN portal.compliance_assets existing ON existing.asset_path=current_asset.asset_path AND existing.sha256=current_asset.sha256;")
    sql.append(scan_sql)
    # Retain old hashes as history, but mark records no longer present in the
    # current scan as inactive instead of silently deleting their evidence.
    if assets:
        sql.append("UPDATE portal.compliance_assets old SET published_or_active=FALSE,distribution_status='not_distributed',notes=CASE WHEN old.notes='' THEN 'Superseded or absent from latest inventory scan; retained for history.' ELSE old.notes || ' | Superseded or absent from latest inventory scan; retained for history.' END,updated_at=now() WHERE old.discovered_by IN ('filesystem_scan','package_lock_scan') AND old.published_or_active=TRUE AND NOT EXISTS (SELECT 1 FROM "+current_source+" AS current_asset(asset_path,sha256) WHERE current_asset.asset_path=old.asset_path AND current_asset.sha256=old.sha256);")
    else:
        sql.append('-- Empty inventory: existing assets are left unchanged to prevent accidental mass deactivation.')
    for a in assets:
        lic=a.get('license_key'); lic_expr='(SELECT license_id FROM portal.compliance_license_catalog WHERE spdx_id='+sqlq(lic[0])+' AND license_version='+sqlq(lic[1])+')' if lic else 'NULL'
        vals=[sqlq(a['asset_type']),sqlq(a['asset_name']),sqlq(a['asset_path']),sqlq(a['source_url']),sqlq(a['creator_or_rightsholder']),sqlq(a['ownership_basis']),lic_expr,sqlq(a['permission_status']),sqlq(a['use_scope']),sqlq(a['distribution_status']),sqlq(a['review_status']),sqlq(a['risk_level']),sqlq(a['sha256']),sqlq(a['file_size_bytes']),sqlq(a['discovered_by']),sqlq(a['published_or_active']),sqljson(a['metadata']),sqlq(a['notes'])]
        sql.append('INSERT INTO portal.compliance_assets(asset_type,asset_name,asset_path,source_url,creator_or_rightsholder,ownership_basis,license_id,permission_status,use_scope,distribution_status,review_status,risk_level,sha256,file_size_bytes,discovered_by,published_or_active,metadata,notes) VALUES ('+','.join(vals)+') ON CONFLICT (asset_path,sha256) WHERE asset_path <> \'\' DO UPDATE SET asset_type=EXCLUDED.asset_type,asset_name=EXCLUDED.asset_name,source_url=EXCLUDED.source_url,license_id=COALESCE(EXCLUDED.license_id,portal.compliance_assets.license_id),permission_status=CASE WHEN portal.compliance_assets.permission_status IN (\'confirmed\',\'conditional\') THEN portal.compliance_assets.permission_status ELSE EXCLUDED.permission_status END,distribution_status=EXCLUDED.distribution_status,risk_level=CASE WHEN portal.compliance_assets.review_status IN (\'approved\',\'approved_with_conditions\') THEN portal.compliance_assets.risk_level ELSE EXCLUDED.risk_level END,metadata=portal.compliance_assets.metadata || EXCLUDED.metadata,notes=CASE WHEN length(portal.compliance_assets.notes)=0 THEN EXCLUDED.notes ELSE portal.compliance_assets.notes END,last_scanned_at=now(),updated_at=now();')
    for e in evidence:
        sql.append('INSERT INTO portal.compliance_asset_evidence(asset_id,evidence_type,evidence_uri,evidence_text,sha256,verified,notes) SELECT a.asset_id,'+','.join([sqlq(e['evidence_type']),sqlq(e['evidence_uri']),sqlq(e['evidence_text']),sqlq(e['sha256']),sqlq(e['verified']),sqlq(e['notes'])])+" FROM portal.compliance_assets a WHERE a.asset_path="+sqlq(e['asset_path'])+" AND NOT EXISTS (SELECT 1 FROM portal.compliance_asset_evidence old WHERE old.asset_id=a.asset_id AND old.evidence_type="+sqlq(e['evidence_type'])+" AND old.evidence_uri="+sqlq(e['evidence_uri'])+" AND old.sha256="+sqlq(e['sha256'])+") LIMIT 1;")
    for a in assets:
        if a['risk_level'] in ('high','critical'):
            decision='pending_evidence'; findings=a['notes']
            sql.append('INSERT INTO portal.compliance_asset_reviews(asset_id,reviewer,review_type,decision,risk_level,findings,required_actions,evidence_snapshot) SELECT a.asset_id,\'automated-inventory\',\'license\',\'pending_evidence\','+sqlq(a['risk_level'])+','+sqlq(findings)+','+sqljson(['verify the package license and source','confirm applicable license conditions and notices','complete a manual license review before redistribution'])+','+sqljson(a['metadata'])+' FROM portal.compliance_assets a WHERE a.asset_path='+sqlq(a['asset_path'])+' AND NOT EXISTS (SELECT 1 FROM portal.compliance_asset_reviews old WHERE old.asset_id=a.asset_id AND old.reviewer=\'automated-inventory\' AND old.review_type=\'license\' AND old.decision=\'pending_evidence\' AND old.findings='+sqlq(findings)+') LIMIT 1;')
    sql.append('COMMIT;')
    (out/'seed-compliance-inventory.sql').write_text('\n'.join(sql)+'\n')
    counts={k:sum(1 for a in assets if a['asset_type']==k) for k in sorted({a['asset_type'] for a in assets})}
    risk_counts={k:sum(1 for a in assets if a['risk_level']==k) for k in ('critical','high','medium','low','unknown')}
    report=f'''# 个人门户自身软件与静态资源许可证盘点

- 生成时间：{now}
- Git 提交：{source_revision}
- 工作树未提交变更：{'是' if source_worktree_dirty else '否'}（若为是，扫描内容不能仅由提交号完全复现）
- 扫描范围：个人门户自身源代码、公开静态文件、当前发布版本、第三方依赖闭包及其许可证文件/声明。
- 明确排除：用户发布的文章、教育资源、上传图片和外部链接；这些业务内容不进入本合规扫描台账。
- 生产依赖闭包：{len(prod)} 个 package instance（不是唯一软件项目数量）。
- 许可证目录候选：{len(licenses)} 条；所有记录均保持 `unreviewed`，自动扫描不代表法律批准。
- 门户自身资产记录：{len(assets)} 条。
- 风险分布：{json.dumps(risk_counts,ensure_ascii=False)}。
- 资产类型分布：{json.dumps(counts,ensure_ascii=False)}。
- 缺少许可证 metadata 的生产依赖：{len(missing_metadata)} 个。
- 找不到本地许可证文件的生产依赖：{len(missing_license_file)} 个。
- 未在当前第三方声明文件中找到同名版本章节的依赖：{len(missing_notice)} 个。

## 人工核验重点

1. 对标记为高风险或未知的生产依赖，核对其实际打包情况、许可证全文、通知文件覆盖范围和分发义务。
2. 对门户自身静态资源和源码记录，确认贡献者/权利归属及部署产物与当前源码版本的一致性。
3. 自动扫描仅收集门户开发与部署所用的软件/资源证据，不对用户发布内容作版权判断。

## 许可证声明现状

- 当前 `frontend/public/THIRD_PARTY_NOTICES.txt` 存在，已收录 Bootstrap Icons MIT 声明及生产依赖许可证文本。
- 许可证 metadata、文件存在、MIT 等常见宽松许可证并不自动等于实际分发义务已满足；每个资产仍需结合构建产物和许可证条款复核。
- 本轮仅盘点门户自身开发/部署所用资源；不会扫描、登记或修改用户发布的文章、教育资源和上传图片。

## 机器生成文件

- `asset-inventory.csv`：门户自身资产台账导出。
- `asset-inventory.json`：完整扫描结果。
- `third-party-license-register.csv`：生产依赖许可证登记。
- `seed-compliance-inventory.sql`：将门户自身资产、许可证候选和依赖证据导入专有数据库表。

**限制：** 自动扫描无法证明贡献者身份、授权链、合同有效性或实际分发的完整法律结论。所有未知项继续标记为待核实。
'''
    (out/'portal-development-asset-inventory.md').write_text(report)
    summary={'source_revision':source_revision,'source_worktree_dirty':source_worktree_dirty,'assets':len(assets),'asset_types':counts,'risk_counts':risk_counts,'production_dependencies':len(prod),'license_catalog_candidates':len(licenses),'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file,'missing_notice_sections':missing_notice,'seed_sql':str(out/'seed-compliance-inventory.sql'),'report':str(out/'portal-development-asset-inventory.md')}
    print(json.dumps(summary,ensure_ascii=False,indent=2))
    if args.strict_licenses and (missing_metadata or missing_license_file or missing_notice):
        raise SystemExit('Strict license preflight failed: missing metadata/files/notices are listed in the generated reports.')
if __name__=='__main__': main()
