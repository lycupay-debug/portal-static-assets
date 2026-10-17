#!/usr/bin/env python3
"""Build a reproducible first-party portal copyright/license inventory.

This is an evidence collector, not an automated legal clearance tool. Unknown
ownership and permission are deliberately retained as unknown.
"""
from __future__ import annotations
import argparse, csv, hashlib, json, os, re, shlex
from pathlib import Path, PurePosixPath
from datetime import datetime, timezone

EXT_TYPES = {
    '.svg': 'icon', '.png': 'image', '.jpg': 'image', '.jpeg': 'image', '.webp': 'image', '.gif': 'image', '.avif': 'image', '.ico': 'image',
    '.woff': 'font', '.woff2': 'font', '.ttf': 'font', '.otf': 'font', '.eot': 'font',
    '.mp3': 'audio', '.wav': 'audio', '.ogg': 'audio', '.mp4': 'video', '.webm': 'video',
    '.css': 'source_file', '.js': 'source_file', '.mjs': 'source_file', '.ts': 'source_file', '.vue': 'source_file', '.html': 'source_file', '.json': 'source_file', '.txt': 'policy_document',
    '.docx': 'education_resource', '.pdf': 'education_resource', '.pptx': 'education_resource', '.xlsx': 'education_resource',
}
LICENSE_NAMES = {'license','license.md','license.txt','license.rst','copying','copying.md','notice','notice.md','notice.txt','license-mit','license-apache'}

def sha_bytes(b: bytes) -> str: return hashlib.sha256(b).hexdigest()
def sha_file(p: Path) -> str:
    h=hashlib.sha256()
    with p.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024), b''): h.update(chunk)
    return h.hexdigest()
def read_id3_tags(p: Path):
    try: data=p.read_bytes()
    except OSError: return {}
    if len(data)<10 or data[:3] != b'ID3': return {}
    size=sum((data[6+i]&0x7f) << (7*(3-i)) for i in range(4))
    end=min(len(data),10+size); pos=10; tags={}
    while pos+10 <= end:
        frame_id=data[pos:pos+4].decode('ascii','replace')
        frame_size=sum((data[pos+4+i]&0x7f) << (7*(3-i)) for i in range(4))
        if not frame_id.strip(chr(0)) or frame_size <= 0 or pos+10+frame_size > end: break
        raw=data[pos+10:pos+10+frame_size]
        if frame_id in ('TIT2','TPE1','TALB','TSSE','TXXX') and raw:
            enc=raw[0]; payload=raw[1:]
            codec={0:'latin1',1:'utf-16',2:'utf-16be',3:'utf-8'}.get(enc,'utf-8')
            tags[frame_id]=payload.decode(codec,errors='replace').replace(chr(0),' ').strip()[:800]
        pos += 10+frame_size
    return tags

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
    try: files=[p for p in pkgdir.iterdir() if p.is_file() and p.name.lower() in LICENSE_NAMES]
    except OSError: return None
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
    ap.add_argument('--repo',default='/opt/personal-portal'); ap.add_argument('--db-json',default='/tmp/personal-portal-compliance-db.json')
    ap.add_argument('--release',default='/usr/local/www/personal-portal/current'); ap.add_argument('--uploads',default='/usr/local/www/personal-portal/uploads')
    ap.add_argument('--resources',default='/var/db/personal-portal/resources'); ap.add_argument('--out',default='docs/compliance'); ap.add_argument('--strict-licenses',action='store_true')
    args=ap.parse_args(); repo=Path(args.repo); out=repo/args.out; out.mkdir(parents=True,exist_ok=True)
    try: report_path=str((out/'asset-inventory.json').relative_to(repo))
    except ValueError: report_path=str(out/'asset-inventory.json')
    now=datetime.now(timezone.utc).isoformat(); dbdata=json.loads(Path(args.db_json).read_text()) if Path(args.db_json).exists() else {'content':[],'resources':[]}
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
    def add_file(p: Path, path_label: str, distribution: str, active: bool, discovered='filesystem_scan'):
        try:
            if not p.is_file(): return
            st=p.stat(); ext=p.suffix.lower(); typ=EXT_TYPES.get(ext,'other')
            if 'icons/bootstrap-icons/' in path_label.replace('\\','/') and ext=='.svg': typ='icon'
            sha=sha_file(p)
            lic_key=None; own='unknown'; perm='unknown'; risk='unknown'; notes='Automated file inventory; ownership and authorization have not been legally verified.'
            meta={'extension':ext,'scan_timestamp':now,'source_relative_path':path_label,'sha256_method':'SHA-256'}
            if typ=='icon' and 'icons/bootstrap-icons/' in path_label.replace('\\','/'):
                lic_key=add_license('MIT',name='MIT License',notes='Bootstrap Icons artwork; exact notice preserved in frontend/public/THIRD_PARTY_NOTICES.txt')
                own='open_license'; perm='conditional'; risk='low'; meta.update({'upstream':'twbs/icons','upstream_version':'1.13.2','notice_path':'frontend/public/THIRD_PARTY_NOTICES.txt'})
                notes='MIT license declared in bundled third-party notices; verify notice remains included in every distribution.'
            elif typ=='source_file' and path_label.startswith('frontend/src/'):
                own='original'; risk='unknown'; meta['source_role']='first_party_project_source'; notes='Project source file; legal rights holder and contributor chain not independently verified.'
            assets.append(make_asset(asset_type=typ,asset_name=p.name,asset_path=path_label,ownership_basis=own,license_key=lic_key,permission_status=perm,distribution_status=distribution,review_status='pending',risk_level=risk,sha256=sha,file_size_bytes=st.st_size,discovered_by=discovered,published_or_active=active,metadata=meta,notes=notes))
        except (OSError,PermissionError) as e:
            assets.append(make_asset(asset_name=p.name,asset_path=path_label,discovered_by=discovered,metadata={'scan_error':str(e)},notes='File could not be fully inspected; requires manual follow-up.'))
    # Project-owned active source files; exclude historical .bak copies from active asset counts.
    src=repo/'frontend/src'
    for p in sorted(src.rglob('*')):
        if p.is_file() and '.bak-' not in p.name and not p.name.endswith('.bak') and p.suffix.lower() in {'.svg','.png','.jpg','.jpeg','.webp','.gif','.ico','.woff','.woff2','.ttf','.otf','.css','.js','.mjs','.ts','.vue','.html','.json'}:
            add_file(p, str(p.relative_to(repo)), 'unknown', False)
    # Public source assets and currently deployed assets.
    for base, label, dist, active in [(repo/'frontend/public','frontend/public','bundled_in_frontend',True),(Path(args.release),'deployed/current','served_publicly',True),(Path(args.uploads),'uploads','served_publicly',True),(Path(args.resources),'resources','served_publicly',True)]:
        if not base.exists(): continue
        for p in sorted(base.rglob('*')):
            if p.is_file():
                try: rel=p.relative_to(base).as_posix()
                except ValueError: rel=p.name
                add_file(p, f'{label}/{rel}', dist, active, 'filesystem_scan')
    # Dependency license records from the lockfile, restricted to production closure.
    lock=json.loads((repo/'frontend/package-lock.json').read_text()); packages, prod=production_closure(lock)
    notice=(repo/'frontend/public/THIRD_PARTY_NOTICES.txt').read_text(errors='replace') if (repo/'frontend/public/THIRD_PARTY_NOTICES.txt').exists() else ''
    notice_sections=set(re.findall(r'^### (.+?)@([^\n]+)$',notice,re.M))
    missing_metadata=[]; missing_license_file=[]; missing_notice=[]
    for key in sorted(prod):
        meta=packages[key]; name=package_name(key); ver=meta.get('version',''); expr=meta.get('license')
        if isinstance(expr,dict): expr=expr.get('type') or json.dumps(expr,ensure_ascii=False)
        if isinstance(expr,list): expr=' OR '.join(str(x) for x in expr)
        pkgdir=repo/'frontend'/key; licfile=find_license_file(pkgdir); lictext=''
        if licfile:
            try: lictext=licfile.read_text(errors='replace')[:200000]
            except OSError: lictext=''
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
    # Live public database content: do not assume user-submitted content is licensed.
    external_image_urls=set()
    for c in dbdata.get('content',[]):
        body=c.get('body') or ''; summary=c.get('summary') or ''
        urls=set(re.findall(r'https?://[^\s)\]<>"\']+',body+' '+summary))
        image_urls={u.rstrip('.,') for u in urls if '/_next/image' in u or 'cdn.sanity.io/images/' in u or re.search(r'\.(?:png|jpe?g|webp|gif|svg|avif)(?:\?|$)',u,re.I)}
        external_image_urls.update(image_urls)
        high=c.get('id')=='168d654e-2fec-4133-bfa0-87f50581d7e2'
        assets.append(make_asset(asset_type='article',asset_name=c.get('title') or '(untitled article)',asset_path=f"db://portal.portal_content/{c.get('id')}",source_url='https://www.snaperdigital.com/blog/what-is-a-crm' if high else '',creator_or_rightsholder='',ownership_basis='unknown',permission_status='unknown',use_scope='commercial_web',distribution_status='served_publicly' if c.get('status')=='published' and c.get('visibility')=='public' else 'not_distributed',review_status='pending',risk_level='high' if high else 'low' if len(body.strip()) < 100 and len(urls) == 0 else 'medium',sha256=sha_bytes(body.encode()),discovered_by='database_scan',published_or_active=c.get('status')=='published' and c.get('visibility')=='public',metadata={'record_id':c.get('id'),'slug':c.get('slug'),'content_type':c.get('type'),'status':c.get('status'),'visibility':c.get('visibility'),'body_chars':len(body),'summary_chars':len(summary),'external_image_count':len(image_urls),'external_urls_count':len(urls),'copyright_review_finding':'Public body contains translated Snaper Digital navigation, author information, CRM article text and external image URLs; authorization has not been verified.' if high else 'Public content has no recorded source or authorization evidence in the content schema.'},notes='Rights holder, originality and permission are not recorded in the existing content model. Review before making any legal conclusion.'))
        if high:
            evidence.append({'asset_path':f"db://portal.portal_content/{c.get('id')}",'evidence_type':'source_page','evidence_uri':'https://www.snaperdigital.com/blog/what-is-a-crm','evidence_text':f"Automated content review: {len(body)} characters; source-domain links and CDN image URLs are present. This is a risk indicator, not a final infringement determination.",'sha256':sha_bytes(body.encode()),'verified':False,'notes':'Requires original-vs-copy comparison and proof of authorization.'})
    for url in sorted(external_image_urls):
        parent=';'.join(c.get('title','') for c in dbdata.get('content',[]) if url in (c.get('body') or '') or url in (c.get('summary') or ''))
        assets.append(make_asset(asset_type='external_link',asset_name='Externally hosted image: '+url.split('?')[0][-180:],asset_path='external-url://'+sha_bytes(url.encode()),source_url=url,ownership_basis='unknown',permission_status='unknown',use_scope='commercial_web',distribution_status='served_publicly',review_status='pending',risk_level='high' if 'snaperdigital.com' in url or 'cdn.sanity.io' in url else 'unknown',discovered_by='database_scan',published_or_active=True,metadata={'embedded_in_public_content':True,'parent_content_titles':parent,'remote_hosting':True,'authorization_verified':False},notes='Public article embeds an externally hosted image. Verify image rights and hotlink/embedding permission; no authorization inferred from URL availability.'))
    # Publicly exposed education resources, including their stored bytes and metadata.
    for r in dbdata.get('resources',[]):
        stored=r.get('stored_filename') or ''
        resource_path=Path(args.resources)/stored
        exists=resource_path.is_file(); sha=sha_file(resource_path) if exists else ''
        docx='docx' in (r.get('mime_type') or '').lower() or stored.lower().endswith('.docx')
        mp3='audio' in (r.get('mime_type') or '').lower() or stored.lower().endswith('.mp3')
        risk='high' if r.get('is_public') and mp3 else 'medium' if r.get('is_public') and docx else 'unknown'
        audio_tags=read_id3_tags(resource_path) if mp3 and exists else {}
        metadata={'record_id':r.get('id'),'title':r.get('title'),'description':r.get('description'),'original_filename':stored,'display_filename':r.get('original_filename'),'mime_type':r.get('mime_type'),'file_exists':exists,'file_hash_verified':bool(sha),'resource_type':r.get('resource_type'),'external_url':r.get('resource_url'),'source_url_mismatch_suspected':bool(docx and r.get('resource_url') and 'bootstrap' in r.get('resource_url')),'stored_path':str(resource_path)}
        if audio_tags:
            metadata['id3_tags']=audio_tags
            metadata['aigc_marker_present']=('AIGC' in audio_tags.get('TXXX',''))
        assets.append(make_asset(asset_type='education_resource',asset_name=r.get('original_filename') or r.get('title') or 'resource',asset_path=f"db://portal.teaching_resources/{r.get('id')}",source_url=r.get('resource_url') or '',ownership_basis='unknown',permission_status='unknown',use_scope='commercial_web',distribution_status='served_publicly' if r.get('is_public') else 'not_distributed',review_status='pending',risk_level=risk,sha256=sha,file_size_bytes=r.get('file_size'),discovered_by='database_scan',published_or_active=bool(r.get('is_public')),metadata=metadata,notes='Public education file has no recorded author/license/permission evidence. High-priority review is precautionary, not a finding of infringement.'))
        if exists:
            evidence.append({'asset_path':f"db://portal.teaching_resources/{r.get('id')}",'evidence_type':'hash_record','evidence_uri':str(resource_path),'evidence_text':f"Stored file SHA-256: {sha}; byte size: {resource_path.stat().st_size}. File content/provenance still requires human review.",'sha256':sha,'verified':False,'notes':'Read-only inventory; no file content changed.'})
        if mp3 and audio_tags: evidence.append({'asset_path':f"db://portal.teaching_resources/{r.get('id')}",'evidence_type':'other','evidence_uri':str(resource_path)+'#ID3v2.4','evidence_text':'ID3 metadata captured: '+json.dumps(audio_tags,ensure_ascii=False),'sha256':sha,'verified':False,'notes':'AIGC/audio-synthesis markers are provenance clues only; they do not establish rights in the underlying script, voice or source material.'})
    # Existing third-party notices file is itself a compliance artifact.
    notice_path=repo/'frontend/public/THIRD_PARTY_NOTICES.txt'
    if notice_path.is_file():
        assets.append(make_asset(asset_type='policy_document',asset_name='THIRD_PARTY_NOTICES.txt',asset_path='report://THIRD_PARTY_NOTICES.txt',ownership_basis='original',permission_status='conditional',distribution_status='bundled_in_frontend',review_status='pending',risk_level='low',sha256=sha_file(notice_path),file_size_bytes=notice_path.stat().st_size,discovered_by='filesystem_scan',published_or_active=True,metadata={'notice_sections':len(notice_sections),'production_dependency_closure':len(prod),'missing_notice_sections':missing_notice,'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file},notes='Contains Bootstrap Icons MIT notice and production dependency license text; verify section coverage and source notice fidelity.'))
    # Write inventory exports.
    def clean_asset(a):
        x=dict(a); x['license_key']=list(x['license_key']) if x.get('license_key') else None; return x
    (out/'asset-inventory.json').write_text(json.dumps({'generated_at':now,'assets':[clean_asset(a) for a in assets]},ensure_ascii=False,indent=2)+'\n')
    with (out/'asset-inventory.csv').open('w',newline='',encoding='utf-8-sig') as f:
        fields=['asset_type','asset_name','asset_path','source_url','ownership_basis','permission_status','use_scope','distribution_status','review_status','risk_level','sha256','file_size_bytes','discovered_by','published_or_active','notes','metadata']
        w=csv.DictWriter(f,fieldnames=fields); w.writeheader()
        for a in assets:
            row={k:a.get(k) for k in fields}; row['metadata']=json.dumps(row['metadata'],ensure_ascii=False); w.writerow(row)
    with (out/'third-party-license-register.csv').open('w',newline='',encoding='utf-8-sig') as f:
        fields=['package','version','license_expression','license_file','license_file_sha256','license_text_found','notice_section_found','lockfile_path','risk_triage']; w=csv.DictWriter(f,fieldnames=fields); w.writeheader(); w.writerows(prod_rows)
    # SQL import/upsert script; no license is marked legally approved by automation.
    sql=['-- Generated by scripts/compliance/scan_assets.py. Evidence capture only; not legal clearance.','BEGIN;']
    for l in licenses.values():
        sql.append(
            'INSERT INTO portal.compliance_license_catalog (spdx_id,license_name,license_version,license_family,canonical_url,full_text,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status,notes) VALUES (' + ','.join([sqlq(l['spdx_id']),sqlq(l['license_name']),sqlq(l['license_version']),sqlq(l['license_family']),sqlq(l['canonical_url']),sqlq(l['full_text']),sqlq(l['commercial_use_allowed']),sqlq(l['modification_allowed']),sqlq(l['redistribution_allowed']),sqlq(l['attribution_required']),sqlq(l['source_code_offer_required']),sqlq('unreviewed'),sqlq(l['notes'])]) +
            ") ON CONFLICT (spdx_id,license_version) DO UPDATE SET canonical_url=CASE WHEN length(portal.compliance_license_catalog.canonical_url)=0 THEN EXCLUDED.canonical_url ELSE portal.compliance_license_catalog.canonical_url END, full_text=CASE WHEN length(portal.compliance_license_catalog.full_text)=0 THEN EXCLUDED.full_text ELSE portal.compliance_license_catalog.full_text END, license_family=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.license_family ELSE portal.compliance_license_catalog.license_family END, commercial_use_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.commercial_use_allowed ELSE portal.compliance_license_catalog.commercial_use_allowed END, modification_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.modification_allowed ELSE portal.compliance_license_catalog.modification_allowed END, redistribution_allowed=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.redistribution_allowed ELSE portal.compliance_license_catalog.redistribution_allowed END, attribution_required=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.attribution_required ELSE portal.compliance_license_catalog.attribution_required END, source_code_offer_required=CASE WHEN portal.compliance_license_catalog.review_status='unreviewed' THEN EXCLUDED.source_code_offer_required ELSE portal.compliance_license_catalog.source_code_offer_required END, notes=CASE WHEN length(portal.compliance_license_catalog.notes)=0 THEN EXCLUDED.notes ELSE portal.compliance_license_catalog.notes END, updated_at=now();"
        )
    current_pairs = ','.join('('+sqlq(a['asset_path'])+','+sqlq(a['sha256'])+')' for a in assets)
    source_revision=os.popen('git -C '+shlex.quote(str(repo))+' rev-parse HEAD').read().strip()
    finding_count=len([a for a in assets if a['risk_level'] in ('high','critical','unknown')])
    scan_summary={'generated_at':now,'production_dependency_count':len(prod),'license_catalog_records':len(licenses),'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file,'missing_notice_sections':missing_notice,'risk_counts':{k:sum(1 for a in assets if a['risk_level']==k) for k in ('critical','high','medium','low','unknown')}}
    scan_sql=("INSERT INTO portal.compliance_scan_runs(scan_type,scan_status,source_revision,assets_seen,assets_created,assets_updated,findings_count,report_path,summary,completed_at) SELECT 'full_audit','completed_with_warnings',"+sqlq(source_revision)+","+str(len(assets))+",COUNT(*) FILTER (WHERE existing.asset_id IS NULL),COUNT(*) FILTER (WHERE existing.asset_id IS NOT NULL),"+str(finding_count)+","+sqlq(report_path)+","+sqlq(json.dumps(scan_summary,ensure_ascii=False))+"::jsonb,now() FROM (VALUES "+current_pairs+") AS current_asset(asset_path,sha256) LEFT JOIN portal.compliance_assets existing ON existing.asset_path=current_asset.asset_path AND existing.sha256=current_asset.sha256;")
    sql.append(scan_sql)
    # Retain old hashes as history, but mark records no longer present in the
    # current scan as inactive instead of silently deleting their evidence.
    sql.append("UPDATE portal.compliance_assets old SET published_or_active=FALSE,distribution_status='not_distributed',notes=CASE WHEN old.notes='' THEN 'Superseded or absent from latest inventory scan; retained for history.' ELSE old.notes || ' | Superseded or absent from latest inventory scan; retained for history.' END,updated_at=now() WHERE old.discovered_by IN ('filesystem_scan','package_lock_scan','database_scan') AND old.published_or_active=TRUE AND NOT EXISTS (SELECT 1 FROM (VALUES "+current_pairs+") AS current_asset(asset_path,sha256) WHERE current_asset.asset_path=old.asset_path AND current_asset.sha256=old.sha256);")
    for a in assets:
        lic=a.get('license_key'); lic_expr='(SELECT license_id FROM portal.compliance_license_catalog WHERE spdx_id='+sqlq(lic[0])+' AND license_version='+sqlq(lic[1])+')' if lic else 'NULL'
        vals=[sqlq(a['asset_type']),sqlq(a['asset_name']),sqlq(a['asset_path']),sqlq(a['source_url']),sqlq(a['creator_or_rightsholder']),sqlq(a['ownership_basis']),lic_expr,sqlq(a['permission_status']),sqlq(a['use_scope']),sqlq(a['distribution_status']),sqlq(a['review_status']),sqlq(a['risk_level']),sqlq(a['sha256']),sqlq(a['file_size_bytes']),sqlq(a['discovered_by']),sqlq(a['published_or_active']),sqljson(a['metadata']),sqlq(a['notes'])]
        sql.append('INSERT INTO portal.compliance_assets(asset_type,asset_name,asset_path,source_url,creator_or_rightsholder,ownership_basis,license_id,permission_status,use_scope,distribution_status,review_status,risk_level,sha256,file_size_bytes,discovered_by,published_or_active,metadata,notes) VALUES ('+','.join(vals)+') ON CONFLICT (asset_path,sha256) WHERE asset_path <> \'\' DO UPDATE SET asset_type=EXCLUDED.asset_type,asset_name=EXCLUDED.asset_name,source_url=EXCLUDED.source_url,license_id=COALESCE(EXCLUDED.license_id,portal.compliance_assets.license_id),permission_status=CASE WHEN portal.compliance_assets.permission_status IN (\'confirmed\',\'conditional\') THEN portal.compliance_assets.permission_status ELSE EXCLUDED.permission_status END,distribution_status=EXCLUDED.distribution_status,risk_level=CASE WHEN portal.compliance_assets.review_status IN (\'approved\',\'approved_with_conditions\') THEN portal.compliance_assets.risk_level ELSE EXCLUDED.risk_level END,metadata=portal.compliance_assets.metadata || EXCLUDED.metadata,notes=CASE WHEN length(portal.compliance_assets.notes)=0 THEN EXCLUDED.notes ELSE portal.compliance_assets.notes END,last_scanned_at=now(),updated_at=now();')
    for e in evidence:
        sql.append('INSERT INTO portal.compliance_asset_evidence(asset_id,evidence_type,evidence_uri,evidence_text,sha256,verified,notes) SELECT a.asset_id,'+','.join([sqlq(e['evidence_type']),sqlq(e['evidence_uri']),sqlq(e['evidence_text']),sqlq(e['sha256']),sqlq(e['verified']),sqlq(e['notes'])])+" FROM portal.compliance_assets a WHERE a.asset_path="+sqlq(e['asset_path'])+" AND NOT EXISTS (SELECT 1 FROM portal.compliance_asset_evidence old WHERE old.asset_id=a.asset_id AND old.evidence_type="+sqlq(e['evidence_type'])+" AND old.evidence_uri="+sqlq(e['evidence_uri'])+" AND old.sha256="+sqlq(e['sha256'])+") LIMIT 1;")
    for a in assets:
        if a['risk_level'] in ('high','critical'):
            decision='pending_evidence'; findings=a['notes']
            sql.append('INSERT INTO portal.compliance_asset_reviews(asset_id,reviewer,review_type,decision,risk_level,findings,required_actions,evidence_snapshot) SELECT a.asset_id,\'automated-inventory\',\'copyright\',\'pending_evidence\','+sqlq(a['risk_level'])+','+sqlq(findings)+','+sqljson(['verify rightsholder and source','obtain written permission or valid license','if authorization cannot be confirmed, replace or unpublish after owner review'])+','+sqljson(a['metadata'])+' FROM portal.compliance_assets a WHERE a.asset_path='+sqlq(a['asset_path'])+' AND NOT EXISTS (SELECT 1 FROM portal.compliance_asset_reviews old WHERE old.asset_id=a.asset_id AND old.reviewer=\'automated-inventory\' AND old.review_type=\'copyright\' AND old.decision=\'pending_evidence\' AND old.findings='+sqlq(findings)+') LIMIT 1;')
    sql.append('COMMIT;')
    (out/'seed-compliance-inventory.sql').write_text('\n'.join(sql)+'\n')
    counts={k:sum(1 for a in assets if a['asset_type']==k) for k in sorted({a['asset_type'] for a in assets})}
    risk_counts={k:sum(1 for a in assets if a['risk_level']==k) for k in ('critical','high','medium','low','unknown')}
    report=f'''# 版权资产首轮盘点记录\n\n- 生成时间：{now}\n- 扫描范围：活动前端源文件、公开静态文件、当前发布版本、上传图片、教育资源文件、生产依赖闭包、公开文章/资源数据库记录及文章内远程图片链接。\n- 生产依赖闭包：{len(prod)} 个 package instance（不是唯一软件项目数量）。\n- 许可证目录候选：{len(licenses)} 条；所有记录均保持 `unreviewed`，自动扫描不代表法律批准。\n- 资产记录：{len(assets)} 条。\n- 风险分布：{json.dumps(risk_counts,ensure_ascii=False)}。\n- 资产类型分布：{json.dumps(counts,ensure_ascii=False)}。\n- 缺少许可证 metadata 的生产依赖：{len(missing_metadata)} 个。\n- 找不到本地许可证文件的生产依赖：{len(missing_license_file)} 个。\n- 未在当前第三方声明文件中找到同名版本章节的依赖：{len(missing_notice)} 个。\n\n## 高优先级人工核验项\n\n1. 公开文章“阿松大”：已定位对应原文 https://www.snaperdigital.com/blog/what-is-a-crm（标题为 What Is a CRM? Why Growing Businesses Need One，页面标注作者 Zivojin Sreckovic，发布日期 2026-10-09）。网站正文包含对应的中文文章内容、作者资料、导航和外部图片链接；授权未证实，属于高优先级版权风险，尚非最终法律结论。\n2. 公开 DOCX“第一个月（第1-4周）每日落地执行指南”：约 12.7 MB。文档内容呈现为详细的课程执行计划，当前未发现整本教材扫描内容；但数据库未记录作者/许可/授权证据，关联 URL 指向 Bootstrap opacity 文档，来源对应关系需核实。列为中风险待核实，不等于认定侵权。\n3. 公开 MP3“Unit 5 Part B.mp3”：数据库未记录来源、版权方或授权证据。ID3v2.4 中存在 AIGC / audio_synthesis 标记，提示音频可能经过合成，但不能证明底稿、文本、声音或来源素材已获授权。仍应核实后再决定继续公开。\n4. 公开文章中发现的远程图片 URL 已作为单独资产登记；浏览器可以访问图片不等于获得商业展示授权。\n\n## 许可证声明现状\n\n- 当前 `frontend/public/THIRD_PARTY_NOTICES.txt` 存在，已收录 Bootstrap Icons MIT 声明及生产依赖许可证文本；本次会将依赖闭包、具体本地许可证文件路径和 SHA-256 记录到台账。\n- 许可证 metadata、文件存在、MIT 等常见宽松许可证并不自动等于实际分发义务已满足；每个资产仍需结合构建产物和许可证条款复核。\n- 本轮未删除或修改任何公开内容、资源文件或第三方依赖。\n\n## 机器生成文件\n\n- `asset-inventory.csv`：资产台账导出。\n- `asset-inventory.json`：完整扫描结果。\n- `third-party-license-register.csv`：生产依赖许可证登记。\n- `seed-compliance-inventory.sql`：将资产、许可证候选、证据和高风险待审记录导入专有数据库表。\n\n**限制：** 自动扫描无法证明创作人身份、授权链、合同有效性、版权例外适用性或实际分发的完整法律结论。所有未知项继续标记为待核实。\n'''
    (out/'copyright-asset-inventory-2026-10-17.md').write_text(report)
    summary={'assets':len(assets),'asset_types':counts,'risk_counts':risk_counts,'production_dependencies':len(prod),'license_catalog_candidates':len(licenses),'missing_license_metadata':missing_metadata,'missing_license_files':missing_license_file,'missing_notice_sections':missing_notice,'external_images':len(external_image_urls),'seed_sql':str(out/'seed-compliance-inventory.sql'),'report':str(out/'copyright-asset-inventory-2026-10-17.md')}
    print(json.dumps(summary,ensure_ascii=False,indent=2))
    if args.strict_licenses and (missing_metadata or missing_license_file or missing_notice):
        raise SystemExit('Strict license preflight failed: missing metadata/files/notices are listed in the generated reports.')
if __name__=='__main__': main()
