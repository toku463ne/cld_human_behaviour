<?xml version="1.0" encoding="UTF-8"?>
<tileset version="1.10" tiledversion="1.12.2" name="mononoke terrain" tilewidth="32" tileheight="32" tilecount="9" columns="9">
 <image source="terrain.png" width="288" height="32"/>
 <tile id="0" class="field">
  <properties>
   <property name="kind" type="string" value="flat"/>
  </properties>
 </tile>
 <tile id="1" class="broken">
  <properties>
   <property name="kind" type="string" value="rough"/>
  </properties>
 </tile>
 <tile id="2" class="stream">
  <properties>
   <property name="kind" type="string" value="water"/>
  </properties>
 </tile>
 <tile id="3" class="ramp 1">
  <properties>
   <property name="height" type="int" value="1"/>
   <property name="kind" type="string" value="slope"/>
  </properties>
 </tile>
 <tile id="4" class="ledge 1">
  <properties>
   <property name="height" type="int" value="1"/>
   <property name="kind" type="string" value="high"/>
  </properties>
 </tile>
 <tile id="5" class="ledge 2">
  <properties>
   <property name="height" type="int" value="2"/>
   <property name="kind" type="string" value="high"/>
  </properties>
 </tile>
 <tile id="6" class="tree">
  <properties>
   <property name="height" type="int" value="3"/>
   <property name="kind" type="string" value="high"/>
  </properties>
 </tile>
 <tile id="7" class="boulder">
  <properties>
   <property name="height" type="int" value="5"/>
   <property name="kind" type="string" value="high"/>
  </properties>
 </tile>
 <tile id="8" class="ramp 2">
  <properties>
   <property name="height" type="int" value="2"/>
   <property name="kind" type="string" value="slope"/>
  </properties>
 </tile>
</tileset>
