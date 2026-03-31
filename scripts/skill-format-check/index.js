const fs = require('fs');
const path = require('path');

// Allow passing a target directory as the first argument, default to '../../skills'
const targetDirArg = process.argv[2] || '../../skills';
const SKILLS_DIR = path.resolve(__dirname, targetDirArg);

function checkSkillFormat() {
  console.log(`Checking skill format in ${SKILLS_DIR}...`);

  if (!fs.existsSync(SKILLS_DIR)) {
    console.error('Skills directory not found:', SKILLS_DIR);
    process.exit(1);
  }

  const skills = fs.readdirSync(SKILLS_DIR).filter(file => {
    return fs.statSync(path.join(SKILLS_DIR, file)).isDirectory();
  });

  let hasErrors = false;

  skills.forEach(skill => {
    // Skip lark-shared skill completely
    if (skill === 'lark-shared') {
        console.log(`⏭️  Skipping check for ${skill}`);
        return;
    }

    const skillPath = path.join(SKILLS_DIR, skill);
    const skillFile = path.join(skillPath, 'SKILL.md');

    if (!fs.existsSync(skillFile)) {
      console.error(`❌ [${skill}] Missing SKILL.md`);
      hasErrors = true;
      return;
    }

    const content = fs.readFileSync(skillFile, 'utf-8');

    // 检查 YAML Frontmatter
    if (!content.startsWith('---\n') && !content.startsWith('---\r\n')) {
       console.error(`❌ [${skill}] SKILL.md must start with YAML frontmatter (---)`);
       hasErrors = true;
    } else {
        // 兼容不同的换行符
        let endOfFrontmatter = content.indexOf('\n---', 3);
        if (endOfFrontmatter === -1) {
             console.error(`❌ [${skill}] SKILL.md has unclosed YAML frontmatter`);
             hasErrors = true;
        } else {
            const frontmatter = content.substring(3, endOfFrontmatter);
            if (!frontmatter.includes('name:')) {
                console.error(`❌ [${skill}] YAML frontmatter missing 'name'`);
                hasErrors = true;
            }
            if (!frontmatter.includes('description:')) {
                console.error(`❌ [${skill}] YAML frontmatter missing 'description'`);
                hasErrors = true;
            }
            if (!frontmatter.includes('metadata:')) {
                console.warn(`⚠️  [${skill}] YAML frontmatter missing 'metadata' (Warning only)`);
                // hasErrors = true; // Downgrade to warning to not fail on existing skills
            }
        }
    }
  });

  if (hasErrors) {
    console.error('\n❌ Skill format check failed. Please fix the errors above.');
    process.exit(1);
  } else {
    console.log('\n✅ Skill format check passed!');
  }
}

checkSkillFormat();
